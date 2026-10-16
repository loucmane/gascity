#!/usr/bin/env bash
# Run one check as the dedicated account with only staged tools and fixtures.
set -euo pipefail
if [[ $(id -un) != vfugtest ]]; then
  echo 'qualification requires vfugtest; use sudo -u vfugtest scripts/vfug-test-run.sh' >&2
  exit 2
fi
vfug_passwd_home=$(getent passwd "$(id -un)" | cut -d: -f6)
case "$vfug_passwd_home" in
  /work/*) ;;
  *) echo 'passwd home must be under /work/' >&2; exit 2;;
esac
case "$(realpath -m "$vfug_passwd_home")" in
  /work/*) ;;
  *) echo 'passwd home must be under /work/' >&2; exit 2;;
esac
vfug_passwd_home=$(realpath -e "$vfug_passwd_home")
vfug_supervisor_home=false
vfug_empty_gitconfig=false
while [[ ${1:-} == --supervisor-home || ${1:-} == --empty-gitconfig ]]; do
  case $1 in
    --supervisor-home) vfug_supervisor_home=true ;;
    --empty-gitconfig) vfug_empty_gitconfig=true ;;
  esac
  shift
done
if [[ ${1:-} != -- || $# -lt 2 ]]; then
  echo 'usage: vfug-env.sh [--supervisor-home] [--empty-gitconfig] -- COMMAND [ARG...]' >&2
  exit 2
fi
shift
: "${VFUG_ROOT:?set VFUG_ROOT to the copied run tree}"
: "${VFUG_WORK_ROOT:?set VFUG_WORK_ROOT below the vfugtest passwd home}"
: "${VFUG_GO_ROOT:?set VFUG_GO_ROOT to the staged Go toolchain}"
: "${VFUG_MODCACHE:?set VFUG_MODCACHE to the staged module cache}"
: "${VFUG_TOOLS:?set VFUG_TOOLS to the staged tools}"
: "${VFUG_LINT_BIN:?set VFUG_LINT_BIN to the staged lint/codegen tools}"
case "$(realpath -m "$VFUG_WORK_ROOT")" in
  "$vfug_passwd_home"/*) ;;
  *) echo 'fixture work tree must be under the vfugtest passwd home' >&2; exit 2;;
esac
for vfug_name in VFUG_ROOT VFUG_WORK_ROOT VFUG_GO_ROOT VFUG_MODCACHE VFUG_TOOLS VFUG_LINT_BIN; do
  case "$(realpath -m "${!vfug_name}")" in
    /work/*) ;;
    *) echo "$vfug_name must resolve under /work/" >&2; exit 2;;
  esac
  printf -v "$vfug_name" '%s' "$(realpath -m "${!vfug_name}")"
done
if [[ -n ${VFUG_PAIR_BIN:-} ]]; then
  case "$(realpath -m "$VFUG_PAIR_BIN")" in /work/*) ;; *) echo 'VFUG_PAIR_BIN must resolve under /work/' >&2; exit 2;; esac
  VFUG_PAIR_BIN=$(realpath -m "$VFUG_PAIR_BIN")
fi
# Check both roots on every entry, including direct per-check invocations.
# Share bd project-file classification with staging; per-user .beads is safe.
# -I (isolated): no PYTHONPATH or user site can shadow the helper (Review A8).
python3 -I -B "$(dirname "${BASH_SOURCE[0]}")/vfug_discovery.py" "$VFUG_ROOT" "$VFUG_WORK_ROOT"
mkdir -p "$VFUG_WORK_ROOT"
vfug_fixture=$(mktemp -d -p "$VFUG_WORK_ROOT" e.XXXXXX)
mkdir -p "$vfug_fixture"/{h,t,g,x,c,d,run,gobin,gopath}
chmod 700 "$vfug_fixture/h" "$vfug_fixture/run"
# Same seed as scripts/test-gitconfig-path and the acceptance helper: gc doctor
# requires beads.role, and acceptance passes this file through to its children.
# bd's own suites assert an unconfigured role, so --empty-gitconfig keeps the file
# writable but empty for them (2026-10-16: TestInitPromptRoleConfig).
if "$vfug_empty_gitconfig"; then
  : > "$vfug_fixture/gitconfig"
else
  printf '[user]\n\tname = gc-test\n\temail = gc-test@test.local\n[beads]\n\trole = maintainer\n[safe]\n\tdirectory = *\n' \
    > "$vfug_fixture/gitconfig"
fi
mkdir -p "$VFUG_ROOT/go-cache"
ln -s "$VFUG_ROOT/go-cache" "$vfug_fixture/c/go-build"
vfug_command_home="$vfug_fixture/h"
if "$vfug_supervisor_home"; then
  vfug_command_home="$vfug_passwd_home"
fi
printf 'Fixture: %s (supervisor passwd HOME: %s)\n' "$vfug_fixture" "$vfug_supervisor_home"
exec /usr/bin/env -i \
  HOME="$vfug_command_home" USERPROFILE="$vfug_command_home" \
  GC_HOME="$vfug_fixture/g" DOLT_ROOT_PATH="$vfug_fixture/d" \
  TMPDIR="$vfug_fixture/t" GOTMPDIR="$vfug_fixture/t" \
  XDG_CONFIG_HOME="$vfug_fixture/x" XDG_CACHE_HOME="$vfug_fixture/c" \
  XDG_RUNTIME_DIR="$vfug_fixture/run" CLAUDE_CONFIG_DIR="$vfug_fixture/h/claude" \
  GC_TEST_TMUX_SOCKET_PARENT_ROOT="$vfug_fixture/t" TMUX_TMPDIR="$vfug_fixture/t" \
  BEADS_CENTRAL_CONFIG="$vfug_fixture/absent-central.json" \
  GOPATH="$vfug_fixture/gopath" GOMODCACHE="$VFUG_MODCACHE" GOBIN="$vfug_fixture/gobin" \
  GOENV=off GOTOOLCHAIN=local GOPROXY="file://$VFUG_MODCACHE/cache/download" GOSUMDB=off GOWORK=off \
  GOFLAGS='-mod=readonly -buildvcs=false' \
  GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL="$vfug_fixture/gitconfig" \
  GC_DISABLE_USAGE_METRICS=1 BD_DISABLE_METRICS=1 DO_NOT_TRACK=1 \
  VFUG_WORK_ROOT="$VFUG_WORK_ROOT" \
  PATH="${VFUG_PAIR_BIN:-$VFUG_TOOLS}:$VFUG_TOOLS:$vfug_fixture/gobin:$VFUG_LINT_BIN:$VFUG_GO_ROOT/bin:/usr/local/bin:/usr/bin:/bin" \
  "$@"
