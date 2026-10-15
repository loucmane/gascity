#!/usr/bin/env bash
# Execute one command in a private test process/mount namespace and short HOME.
# This script never falls back to the operator HOME or host service namespace.
set -euo pipefail
vfug_root=/home/loucmane/scratch/ga-vfug
if [[ ${1:-} == --inside ]]; then
  shift
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
  exec "$@"
fi
if [[ ${1:-} != -- ]]; then
  echo 'usage: vfug-env.sh -- COMMAND [ARG...]' >&2
  exit 2
fi
shift
mkdir -p /work/ga-vfug
vfug_fixture=$(mktemp -d -p /work/ga-vfug e.XXXX)
mkdir -p "$vfug_fixture"/{h,t,g,x,c,d,run,masked}
chmod 700 "$vfug_fixture/h" "$vfug_fixture/run"
mkdir -p "$vfug_root/go-cache"
ln -s "$vfug_root/go-cache" "$vfug_fixture/c/go-build"
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
printf 'Fixture namespace: %s\n' "$vfug_fixture"
# Explicit environment: no inherited endpoint, agent session, credentials,
# service-manager or bd workspace configuration enters the command.
# --keep-caps: with --map-current-user the shell is uid 1000 inside the namespace and
# would drop the namespace capabilities on exec, so `mount --make-rprivate /` failed
# ("mount failed: Unknown error 5005", 2026-10-15). Caps stay namespace-scoped.
# Keep comments out of the continued command below: a `#` there ends it.
exec /usr/bin/env -i \
  HOME="$vfug_fixture/h" USERPROFILE="$vfug_fixture/h" \
  GC_HOME="$vfug_fixture/g" DOLT_ROOT_PATH="$vfug_fixture/d" \
  TMPDIR="$vfug_fixture/t" GOTMPDIR="$vfug_fixture/t" \
  XDG_CONFIG_HOME="$vfug_fixture/x" XDG_CACHE_HOME="$vfug_fixture/c" \
  XDG_RUNTIME_DIR="$vfug_fixture/run" CLAUDE_CONFIG_DIR="$vfug_fixture/h/claude" \
  GC_TEST_TMUX_SOCKET_PARENT_ROOT="$vfug_fixture/t" \
  BEADS_CENTRAL_CONFIG="$vfug_fixture/absent-central.json" \
  GOPATH=/home/loucmane/go GOMODCACHE=/home/loucmane/go/pkg/mod \
  GOENV=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off \
  GOFLAGS='-mod=readonly -buildvcs=false' \
  GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
  GC_DISABLE_USAGE_METRICS=1 BD_DISABLE_METRICS=1 DO_NOT_TRACK=1 \
  PATH="${VFUG_PAIR_BIN:-$vfug_root/r9/tools}:$vfug_root/r9/tools:$vfug_root/local-r5/bin:/home/loucmane/.local/share/go/1.26.7/bin:/usr/local/bin:/usr/bin:/bin" \
  unshare --user --map-current-user --keep-caps --mount --pid --fork --mount-proc \
  bash "$0" --inside "$vfug_fixture" "$@"
