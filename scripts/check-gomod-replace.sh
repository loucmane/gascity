#!/usr/bin/env bash
# The single operator-approved fork exception is documented in
# engdocs/contributors/beads-fork-pin.md. All other replacements fail.
set -euo pipefail
exec python3 -B "$(dirname "$0")/gomod_replace_policy.py" "${1:-go.mod}"
