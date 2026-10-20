#!/bin/sh
# The integration suite's responder: Cortex's process runner calls it with the job folder, and it
# answers success without acting on anything, so a test can run it on any case.
set -eu

mkdir -p "$1/output"
cat >"$1/output/output.json" <<'EOF'
{"success": true, "full": {"message": "TheHiveMCP test responder: nothing was done"}, "operations": []}
EOF
