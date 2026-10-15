#!/usr/bin/env bash
# ga-vfug host qualification entry. The operator runs it in a real terminal, naming
# the reviewed SHA-256 of this file:
#   sudo sh -c 'install -D -m 0700 -o root -g root /home/loucmane/scratch/ga-vfug/port-r10/scripts/vfug-host-root.sh /run/vfug-host/entry.sh && exec bash /run/vfug-host/entry.sh MODE SHA256'
#
# Stage 1 (root, host namespace): runs only from that root-owned copy, refuses unless
# the copy has the reviewed hash, and continues from the copy only.
# Stage 2 (root, host namespace): build a private passwd in /run/vfug-host, have the
# OPERATOR create the fixture home (root never chowns or chmods operator-writable
# paths), check it is a real directory, and unshare a private mount + PID namespace.
# Stage 3 (root, private namespace, PID 1): no user namespace, so file ownership
# stays real. Make every mount private, bind the private passwd, make the operator
# home read-only except ~/scratch/ga-vfug and ~/go/pkg/mod, mask ~/.beads, ~/gascity,
# /run/user/1000 and /tmp/tmux-1000 with empty read-only directories, prove that
# layout from /proc/self/mountinfo and with probes, then drop to the operator UID
# with no capabilities and no_new_privs and run scripts/vfug-host.py.
# Host changes: /run/vfug-host (root-only), the operator-created /work/ga-vfug/hr,
# the log ~/scratch/ga-vfug/host-r10.out and the probe ~/scratch/ga-vfug/.vfug-rw-probe.
# Residual, by design: the docker group stays (the Dolt container suites need it, and a
# container could bind-mount host paths); there is no network namespace (Docker), so
# 127.0.0.1 ports stay reachable; the /mnt/* Windows drives and WSL interop stay.
set -euo pipefail
op_uid=1000
op_gid=1000
op_home=/home/loucmane
fixture=/work/ga-vfug/hr
state=/run/vfug-host
log=$op_home/scratch/ga-vfug/host-r10.out
modes='all build bd gc gates acceptance cas safety plan'

as_operator() { setpriv --reuid="$op_uid" --regid="$op_gid" --clear-groups --inh-caps=-all --no-new-privs -- "$@"; }

if [[ ${1:-} == --stage3 ]]; then
  # Only valid as PID 1 of the namespace stage 2 creates.
  [[ $$ -eq 1 && $EUID -eq 0 ]] || { echo "--stage3 runs only inside the private namespace" >&2; exit 2; }
  mode=$2
  runner=$3
  mount --make-rprivate /
  mount --bind "$state/passwd" /etc/passwd
  mount -o remount,bind,ro /etc/passwd
  # Writable islands first (separate mounts), then the read-only home over them.
  mount --bind "$op_home/scratch/ga-vfug" "$op_home/scratch/ga-vfug"
  mount --bind "$op_home/go/pkg/mod" "$op_home/go/pkg/mod"
  mount --rbind "$op_home" "$op_home"
  mount -o remount,bind,ro "$op_home"
  for masked in "$op_home/.beads" "$op_home/gascity" "/run/user/$op_uid" "/tmp/tmux-$op_uid"; do
    if [[ -d $masked && ! -L $masked ]]; then
      mount --bind "$state/empty" "$masked"
      mount -o remount,bind,ro "$masked"
    fi
  done
  # Prove the layout before any test runs.
  python3 -B - "$op_home" "$op_uid" <<'PY' || { echo "ABORT: unexpected mounts under the operator home" >&2; exit 3; }
import sys
home, uid = sys.argv[1], sys.argv[2]
allowed_rw = {home + "/scratch/ga-vfug", home + "/go/pkg/mod"}
masked = {home + "/.beads", home + "/gascity"}
bad = []
for line in open("/proc/self/mountinfo"):
    fields = line.split()
    point, options = fields[4], fields[5].split(",")
    if point != home and not point.startswith(home + "/"):
        continue
    readonly = "ro" in options
    if point == home or point in masked:
        if not readonly:
            bad.append(point + " is writable")
    elif point in allowed_rw:
        pass
    else:
        bad.append(point + " is an unexpected mount")
for item in bad:
    print(item, file=sys.stderr)
sys.exit(1 if bad else 0)
PY
  if as_operator touch "$op_home/.vfug-ro-probe" 2>/dev/null; then
    echo "ABORT: operator home is writable inside the namespace" >&2; exit 3
  fi
  as_operator touch "$op_home/scratch/ga-vfug/.vfug-rw-probe" || { echo "ABORT: ~/scratch/ga-vfug is not writable" >&2; exit 3; }
  [[ $(getent passwd "$op_uid" | cut -d: -f6) == "$fixture/h" ]] || { echo "ABORT: private passwd not in effect" >&2; exit 3; }
  for masked in "$op_home/.beads" "$op_home/gascity" "/run/user/$op_uid" "/tmp/tmux-$op_uid"; do
    [[ ! -e $masked || -z $(ls -A "$masked") ]] || { echo "ABORT: $masked not masked" >&2; exit 3; }
  done
  echo "namespace ready: passwd home $fixture/h, home read-only, live paths masked"
  exec setpriv --reuid="$op_uid" --regid="$op_gid" --init-groups \
    --inh-caps=-all --ambient-caps=-all --bounding-set=-all --no-new-privs -- \
    /usr/bin/env -i HOME="$fixture/h" LANG=C.UTF-8 PATH=/usr/local/bin:/usr/bin:/bin \
    bash -c 'python3 -B "$0" "$1" 2>&1 | tee "$2"; exit "${PIPESTATUS[0]}"' "$runner/vfug-host.py" "$mode" "$log"
fi

if [[ ${1:-} == --stage2 ]]; then
  [[ $EUID -eq 0 && ${BASH_SOURCE[0]} == "$state/entry.sh" ]] || { echo "--stage2 runs only from the verified copy" >&2; exit 2; }
  mode=$2
  runner=$3
  install -d -m 0555 -o root -g root "$state/empty"
  awk -F: -v OFS=: -v uid="$op_uid" -v home="$fixture/h" '$3 == uid { $6 = home; n++ } { print } END { if (n != 1) exit 1 }' \
    /etc/passwd > "$state/passwd" || { echo "operator UID not found exactly once in /etc/passwd" >&2; exit 2; }
  chmod 0644 "$state/passwd"
  # The operator creates its own fixture home; root only checks it.
  as_operator mkdir -p "$fixture/h"
  as_operator chmod 0700 "$fixture/h"
  for path in /work /work/ga-vfug "$fixture" "$fixture/h"; do
    [[ -d $path && ! -L $path ]] || { echo "refusing: $path is not a real directory" >&2; exit 2; }
  done
  [[ $(realpath -e "$fixture/h") == "$fixture/h" ]] || { echo "refusing: $fixture/h resolves elsewhere" >&2; exit 2; }
  [[ $(stat -c %u "$fixture/h") == "$op_uid" ]] || { echo "refusing: $fixture/h is not owned by the operator" >&2; exit 2; }
  echo "log: $log"
  exec unshare --mount --pid --fork --mount-proc --propagation private \
    bash "$state/entry.sh" --stage3 "$mode" "$runner"
fi

# Stage 1: only from the root-owned copy the operator's command installs first, so
# no byte root executes comes from an operator-writable file.
mode=${1:-}
want=${2:-}
usage="usage: sudo sh -c 'install -D -m 0700 -o root -g root <this file> $state/entry.sh && exec bash $state/entry.sh MODE SHA256'"
[[ " $modes " == *" $mode "* && $want =~ ^[0-9a-f]{64}$ ]] || { echo "$usage (MODE: $modes)" >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "run with sudo" >&2; exit 2; }
[[ ${SUDO_UID:-} == "$op_uid" ]] || { echo "run with sudo from the operator account" >&2; exit 2; }
[[ ${BASH_SOURCE[0]} == "$state/entry.sh" ]] || { echo "$usage" >&2; exit 2; }
[[ $(stat -c '%u %a' "$state" "$state/entry.sh" | tr '\n' ' ') == "0 "*" 0 700 " ]] \
  || { echo "refusing: $state/entry.sh must be root-owned" >&2; exit 2; }
got=$(sha256sum < "$state/entry.sh" | cut -c1-64)
[[ $got == "$want" ]] || { echo "refusing: this file's hash $got is not the reviewed $want" >&2; exit 2; }
chmod 0700 "$state"
runner=$op_home/scratch/ga-vfug/port-r10/scripts
exec bash "$state/entry.sh" --stage2 "$mode" "$runner"
