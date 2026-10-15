#!/usr/bin/env bash
# Dedicated vfugtest entry; only verified input copies are executed.
set -euo pipefail
exec python3 -B "$(dirname "${BASH_SOURCE[0]}")/vfug-stage.py" run "$@"
