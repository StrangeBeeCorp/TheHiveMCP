#!/usr/bin/env bash
# Single source of truth for the checkmake step. Both callers use THIS script so
# the image pin, config path, and docker invocation live in exactly one place:
#   • scripts/lint.sh  → check_makefiles() (part of `make lint` / the full gate)
#   • make lint-makefile
#
# Usage: lint-makefile.sh [MAKEFILE...]
#   Paths are relative to the repo root. Defaults to `Makefile` if none given.
#   Prints checkmake's report to stdout; exits non-zero when it finds violations.
set -euo pipefail

# Pinned checkmake. Official image, digest-pinned. Its entrypoint is
# `./checkmake` relative to WORKDIR `/`, so we override it to the absolute
# `/checkmake` (the relative form breaks once we set `-w /app`) and pass the
# config + explicit paths ourselves. Rule config lives in checkmake.ini.
CHECKMAKE_IMAGE="quay.io/checkmake/checkmake@sha256:6db49d6b62e2abed57c2fd334ca5f43067126bd9419a71b13967bd0eccf6cdcb" # v0.3.2 (official)

REPO_ROOT="$(git rev-parse --show-toplevel)"

# Default target is the repo-root Makefile; callers may pass explicit paths.
mk_paths=("$@")
[[ ${#mk_paths[@]} -gt 0 ]] || mk_paths=("Makefile")

docker pull -q "$CHECKMAKE_IMAGE" >/dev/null 2>&1 || true

cfg=() flag=()
if [[ -f "$REPO_ROOT/checkmake.ini" ]]; then
  cfg=(-v "$REPO_ROOT/checkmake.ini":/checkmake.ini:ro)
  flag=(--config=/checkmake.ini)
fi

# A fragment the Makefile includes (*.mk, such as scripts/harness.mk) gets the same rules but
# minphony: the all, clean and test targets that rule asks for are the including Makefile's.
whole=() parts=()
for f in "${mk_paths[@]}"; do
  case "$f" in
    *.mk) parts+=("/app/$f") ;;
    *) whole+=("/app/$f") ;;
  esac
done

status=0
if [[ ${#whole[@]} -gt 0 ]]; then
  docker run --rm --entrypoint /checkmake -v "$REPO_ROOT":/app ${cfg[@]+"${cfg[@]}"} -w /app "$CHECKMAKE_IMAGE" \
    ${flag[@]+"${flag[@]}"} "${whole[@]}" || status=1
fi
if [[ ${#parts[@]} -gt 0 ]]; then
  mkdir -p "$REPO_ROOT/.cache"
  part_cfg="$(mktemp "$REPO_ROOT/.cache/checkmake-fragment.XXXXXX")"
  trap 'rm -f "$part_cfg"' EXIT
  {
    [[ -f "$REPO_ROOT/checkmake.ini" ]] && cat "$REPO_ROOT/checkmake.ini"
    printf '\n[minphony]\ndisabled = true\n'
  } >"$part_cfg"
  docker run --rm --entrypoint /checkmake -v "$REPO_ROOT":/app -v "$part_cfg":/checkmake.ini:ro -w /app "$CHECKMAKE_IMAGE" \
    --config=/checkmake.ini "${parts[@]}" || status=1
fi
exit "$status"
