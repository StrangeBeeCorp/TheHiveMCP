#!/bin/sh
# The integration suite's analyzer: Cortex's process runner calls it with the job folder, and it
# reports the same verdict for any observable without calling anything.
set -eu

mkdir -p "$1/output"
cat >"$1/output/output.json" <<'EOF'
{"success": true, "summary": {"taxonomies": [{"level": "info", "namespace": "TheHiveMCP", "predicate": "Test", "value": "done"}]}, "artifacts": [], "operations": [], "full": {"message": "TheHiveMCP test analyzer: nothing was looked up"}}
EOF
