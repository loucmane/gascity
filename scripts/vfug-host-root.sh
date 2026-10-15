#!/usr/bin/env bash
# ga-vfug host qualification entry. The operator runs it in a real terminal:
#   sudo bash /home/loucmane/scratch/ga-vfug/port-r10/scripts/vfug-host-root.sh [mode]
#
# Root only builds a private mount and PID namespace. There is no user namespace, so
# file ownership looks normal to the tests. Inside it:
#   - /etc/passwd is replaced by a copy whose home for the operator is under
#     /work/ga-vfug, because tests and gc's supervisor code resolve HOME from passwd;
#   - the operator's whole home is read-only, except ~/scratch/ga-vfug (repos, logs,
#     go build cache) and ~/go (module cache); ~/.beads is masked by an empty directory.
# It then drops to the operator UID with no capabilities and no_new_privs and runs
# scripts/vfug-host.py. The mounts vanish with the namespace; the host is not changed
# apart from /run/vfug-host (root-only scratch) and the logs.
set -euo pipefail
op_uid=1000
op_gid=1000
op_home=/home/loucmane
fixture=/work/ga-vfug/hr
state=/run/vfug-host
log=$op_home/scratch/ga-vfug/host-r10.out

if [[ ${1:-} == --inside ]]; then
  mode=$2
  runner=$3
  mount --make-rprivate /
  mount --bind "$state/passwd" /etc/passwd
  mount -o remount,bind,ro /etc/passwd
  # Writable islands first (as separate mounts), then the read-only home over them.
  mount --bind "$op_home/scratch/ga-vfug" "$op_home/scratch/ga-vfug"
  mount --bind "$op_home/go" "$op_home/go"
  mount --rbind "$op_home" "$op_home"
  mount -o remount,bind,ro "$op_home"
  if [[ -d $op_home/.beads ]]; then
    mount --bind "$state/empty" "$op_home/.beads"
    mount -o remount,bind,ro "$op_home/.beads"
  fi
  # Prove the layout before any test runs.
  if setpriv --reuid="$op_uid" --regid="$op_gid" --clear-groups touch "$op_home/.vfug-ro-probe" 2>/dev/null; then
    echo "ABORT: operator home is writable inside the namespace" >&2; exit 3
  fi
  setpriv --reuid="$op_uid" --regid="$op_gid" --clear-groups touch "$op_home/scratch/ga-vfug/.vfug-rw-probe" \
    || { echo "ABORT: ~/scratch/ga-vfug is not writable" >&2; exit 3; }
  [[ $(getent passwd "$op_uid" | cut -d: -f6) == "$fixture/h" ]] \
    || { echo "ABORT: private passwd not in effect" >&2; exit 3; }
  [[ -z $(ls -A "$op_home/.beads" 2>/dev/null) ]] || { echo "ABORT: ~/.beads not masked" >&2; exit 3; }
  echo "namespace ready: passwd home $fixture/h, home read-only, ~/.beads masked"
  exec setpriv --reuid="$op_uid" --regid="$op_gid" --init-groups \
    --inh-caps=-all --ambient-caps=-all --bounding-set=-all --no-new-privs -- \
    /usr/bin/env -i HOME="$fixture/h" LANG=C.UTF-8 PATH=/usr/local/bin:/usr/bin:/bin \
    bash -c 'python3 -B "$0" "$1" 2>&1 | tee "$2"; exit "${PIPESTATUS[0]}"' "$runner/vfug-host.py" "$mode" "$log"
fi

mode=${1:-all}
case $mode in all|build|bd|gc|gates|acceptance|cas|safety|plan) ;; *) echo "unknown mode $mode" >&2; exit 2 ;; esac
[[ $EUID -eq 0 ]] || { echo "run with sudo" >&2; exit 2; }
[[ ${SUDO_UID:-} == "$op_uid" ]] || { echo "run with sudo from the operator account" >&2; exit 2; }
runner=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
[[ $runner == "$op_home/scratch/ga-vfug/"*/scripts ]] || { echo "runner must live under ~/scratch/ga-vfug" >&2; exit 2; }

install -d -m 0700 -o root -g root "$state"
install -d -m 0555 -o root -g root "$state/empty"
awk -F: -v OFS=: -v uid="$op_uid" -v home="$fixture/h" '$3 == uid { $6 = home; n++ } { print } END { if (n != 1) exit 1 }' \
  /etc/passwd > "$state/passwd" || { echo "operator UID not found exactly once in /etc/passwd" >&2; exit 2; }
chmod 0644 "$state/passwd"
install -d -m 0755 -o "$op_uid" -g "$op_gid" /work/ga-vfug "$fixture"
install -d -m 0700 -o "$op_uid" -g "$op_gid" "$fixture/h"
# Run the namespace half from a root-owned copy so an edit during the run cannot change it.
install -m 0700 -o root -g root "${BASH_SOURCE[0]}" "$state/entry.sh"
echo "log: $log"
exec unshare --mount --pid --fork --mount-proc --propagation private \
  bash "$state/entry.sh" --inside "$mode" "$runner"
