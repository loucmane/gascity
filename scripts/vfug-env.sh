#!/usr/bin/env bash
# Execute one command with a scrubbed environment and short fixture HOME.
# --private-home also maps the account home for supervisor acceptance tests.
set -euo pipefail
vfug_root=/home/loucmane/scratch/ga-vfug
if [[ ${1:-} == --inside ]]; then
  shift
  # The namespace we create maps exactly the current UID. Refuse a direct
  # invocation in the initial namespace, including one made as host root.
  if ! awk -v uid="$(id -u)" 'NF == 3 && $1 == uid && $2 == uid && $3 == 1 { mapped++ } END { exit !(NR == 1 && mapped == 1) }' /proc/self/uid_map; then
    echo '--inside requires a private user namespace created by --private-home' >&2
    exit 2
  fi
  vfug_fixture=$1
  shift
  if ! mount --make-rprivate /; then
    echo "Private mounts unavailable: run this runner on a host with unprivileged user/mount namespaces." >&2
    exit 2
  fi
  mount --bind "$vfug_fixture/passwd" /etc/passwd
  # Mask ancestor discovery only in this private mount namespace. No host
  # workspace content is read or written, and no host service is signalled.
  if [[ -d /home/loucmane/.beads ]]; then
    mount --bind "$vfug_fixture/masked" /home/loucmane/.beads
    mount -o remount,bind,ro /home/loucmane/.beads
  fi
  exec setpriv --inh-caps=-all --ambient-caps=-all --bounding-set=-all -- "$@"
fi
vfug_private_home=false
if [[ ${1:-} == --private-home ]]; then
  vfug_private_home=true
  shift
fi
if [[ ${1:-} != -- ]]; then
  echo 'usage: vfug-env.sh [--private-home] -- COMMAND [ARG...]' >&2
  exit 2
fi
shift
# Tests (pinRealHome, pinRealHomeEnv) and gc's supervisor code resolve the account
# home from passwd, not from HOME. Every command must therefore see a passwd home
# under /work/ga-vfug: either from the outer sudo entry (~/scratch/ga-vfug/host-run.sh,
# which binds a private passwd in a private mount namespace without a user namespace)
# or from --private-home below. Never run with the operator's real passwd home
# (Review A3, 2026-10-15).
vfug_passwd_home=$(getent passwd "$(id -u)" | cut -d: -f6)
case "$vfug_passwd_home" in
  /work/ga-vfug/*) vfug_private_home=false ;;
  *)
    if ! "$vfug_private_home"; then
      echo "refusing: passwd home is $vfug_passwd_home; run through host-run.sh (sudo) so tests cannot reach the operator HOME" >&2
      exit 2
    fi ;;
esac
mkdir -p /work/ga-vfug
vfug_fixture=$(mktemp -d -p /work/ga-vfug e.XXXX)
mkdir -p "$vfug_fixture"/{h,t,g,x,c,d,run,masked}
chmod 700 "$vfug_fixture/h" "$vfug_fixture/run"
# A writable fixture-local global git config: gc sets beads.role there when it
# starts a city. /dev/null made every acceptance city fail ("could not lock config
# file /dev/null", 2026-10-15).
: > "$vfug_fixture/gitconfig"
mkdir -p "$vfug_root/go-cache"
ln -s "$vfug_root/go-cache" "$vfug_fixture/c/go-build"
if "$vfug_private_home"; then
  python3 -B - "$vfug_fixture" <<'PY'
import os
from pathlib import Path
import sys
root = Path(sys.argv[1])
uid = str(os.getuid())
rows = []
found = False
for row in Path('/etc/passwd').read_text().splitlines():
    fields = row.split(':')
    if len(fields) == 7 and fields[2] == uid:
        fields[5] = str(root / 'h')
        found = True
    rows.append(':'.join(fields))
if not found:
    raise SystemExit('cannot isolate account HOME: UID absent from passwd')
(root / 'passwd').write_text('\n'.join(rows) + '\n')
PY
  set -- unshare --user --map-current-user --keep-caps --mount --pid --fork --mount-proc \
    bash "$0" --inside "$vfug_fixture" "$@"
fi
printf 'Fixture: %s (private account home: %s)\n' "$vfug_fixture" "$vfug_private_home"
# Explicit environment: no inherited endpoint, agent session, credentials,
# service-manager or bd workspace configuration enters the command.
# --keep-caps permits the private mounts; --inside drops all capabilities
# before executing the command so permission-sensitive tests remain meaningful.
# Keep comments out of the continued command below: a `#` there ends it.
exec /usr/bin/env -i \
  HOME="$vfug_fixture/h" USERPROFILE="$vfug_fixture/h" \
  GC_HOME="$vfug_fixture/g" DOLT_ROOT_PATH="$vfug_fixture/d" \
  TMPDIR="$vfug_fixture/t" GOTMPDIR="$vfug_fixture/t" \
  XDG_CONFIG_HOME="$vfug_fixture/x" XDG_CACHE_HOME="$vfug_fixture/c" \
  XDG_RUNTIME_DIR="$vfug_fixture/run" CLAUDE_CONFIG_DIR="$vfug_fixture/h/claude" \
  GC_TEST_TMUX_SOCKET_PARENT_ROOT="$vfug_fixture/t" \
  TMUX_TMPDIR="$vfug_fixture/t" \
  BEADS_CENTRAL_CONFIG="$vfug_fixture/absent-central.json" \
  GOPATH=/home/loucmane/go GOMODCACHE=/home/loucmane/go/pkg/mod \
  GOENV=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off \
  GOFLAGS='-mod=readonly -buildvcs=false' \
  GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL="$vfug_fixture/gitconfig" \
  GC_DISABLE_USAGE_METRICS=1 BD_DISABLE_METRICS=1 DO_NOT_TRACK=1 \
  PATH="${VFUG_PAIR_BIN:-$vfug_root/r9/tools}:$vfug_root/r9/tools:$vfug_root/local-r5/bin:/home/loucmane/.local/share/go/1.26.7/bin:/usr/local/bin:/usr/bin:/bin" \
  "$@"
