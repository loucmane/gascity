#!/usr/bin/env bash
# Offline round-8 pair: go.mod names the immutable local beads checkout.
# Usage: bash scripts/build-vfug.sh OUTPUT_DIR [LOCAL_BEADS_REPOSITORY]
# Requires Go 1.26.7, a C toolchain, and the dependencies already in GOMODCACHE.
set -euo pipefail

vfug_repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
vfug_output=${1:?usage: build-vfug.sh OUTPUT_DIR [LOCAL_BEADS_REPOSITORY]}
vfug_source=${2:-"$vfug_repo/../bd-src"}
vfug_replace=$(awk '$1 == "replace" && $2 == "github.com/steveyegge/beads" && $3 == "=>" {print $4}' "$vfug_repo/go.mod")
if [[ ! "$vfug_replace" =~ ^\.\./beads-vfug-([0-9a-f]{40})$ ]]; then
  echo 'go.mod must pin ../beads-vfug-<full commit SHA>' >&2
  exit 1
fi
vfug_revision=${BASH_REMATCH[1]}
vfug_pin="$vfug_repo/$vfug_replace"
if [[ ! -e "$vfug_pin" ]]; then
  # --local uses the local object database; it never contacts a remote.
  git clone --local --no-hardlinks --no-checkout "$vfug_source" "$vfug_pin"
  git -C "$vfug_pin" -c core.hooksPath=/dev/null checkout --detach "$vfug_revision"
fi
if [[ "$(git -C "$vfug_pin" rev-parse HEAD)" != "$vfug_revision" ]] ||
   [[ -n "$(git -C "$vfug_pin" status --porcelain --untracked-files=all)" ]]; then
  echo "Pinned beads checkout is dirty or not at $vfug_revision: $vfug_pin" >&2
  exit 1
fi
mkdir -p "$vfug_output"
vfug_output=$(cd "$vfug_output" && pwd)
mkdir -p "$vfug_output/home" "$vfug_output/tmp"
vfug_gopath=${GOPATH:-"$HOME/go"}
vfug_modcache=${GOMODCACHE:-"$vfug_gopath/pkg/mod"}
vfug_cachehome=${XDG_CACHE_HOME:-"$HOME/.cache"}

build() {
  /usr/bin/env -i HOME="$vfug_output/home" PATH="$PATH" \
    XDG_CACHE_HOME="$vfug_cachehome" GOPATH="$vfug_gopath" GOMODCACHE="$vfug_modcache" \
    TMPDIR="$vfug_output/tmp" GOTMPDIR="$vfug_output/tmp" \
    GOENV=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off \
    GOFLAGS='-mod=readonly -buildvcs=false' GOOS=linux GOARCH=amd64 \
    GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
    "$@"
}
if [[ "$(build go version)" != 'go version go1.26.7 linux/amd64' ]]; then
  echo 'Use Go 1.26.7 on linux/amd64 for this recorded build pair' >&2
  exit 1
fi
(
  cd "$vfug_pin"
  build env CGO_ENABLED=1 go build -tags=gms_pure_go -trimpath -o "$vfug_output/bd" ./cmd/bd
)
(
  cd "$vfug_repo"
  build env CGO_ENABLED=0 go build -trimpath -o "$vfug_output/gc" ./cmd/gc
)
{
  printf 'beads_commit=%s\ngc_commit=%s\n' "$vfug_revision" "$(git -C "$vfug_repo" rev-parse HEAD)"
  build go version
  sha256sum "$vfug_output/bd" "$vfug_output/gc"
} > "$vfug_output/inputs.txt"
cat "$vfug_output/inputs.txt"
