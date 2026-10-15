#!/usr/bin/env bash
# Operator-side offline staging; sources are explicit VFUG_*_SOURCE variables.
set -euo pipefail
exec python3 -B "$(dirname "${BASH_SOURCE[0]}")/vfug-stage.py" stage "$@"
