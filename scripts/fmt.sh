#!/usr/bin/env bash
# Single source of truth for repo *formatting* (mutation only — no linters, no
# tests). All formatters run from pinned Docker images — no local toolchain
# required (this is a Go repo with no host Go install; see the Makefile's
# install-dev-deps).
#
# This script owns every formatter the repo runs. lint.sh calls it for the fix
# pass (`lint.sh --fix` → `fmt.sh --fix`), then layers the read-only checkers on
# top; so the format commands and their pinned images live HERE, once, and
# lint.sh never re-implements them. Keeping formatting separate from checking is
# what lets `make fmt` be a pure "rewrite files, never fail" step while `make
# lint` stays a read-only gate.
#
# The CLI signature MIRRORS lint.sh's scope/mode flags so the two are
# interchangeable at the call site:
#   • `make fmt`             → --all --fix           (whole repo, rewrite)
#   • `make fmt-changed`     → --changed --fix        (working-tree files only)
#   • a --check pass          → report files that WOULD be reformatted, exit 1
#
# Formatters, by file type:
#   • Go       — gofmt -w, then golangci-lint fmt (gofumpt/gci/etc. per .golangci.yml)
#   • Shell    — shfmt -i 2 -ci -w
#   • Markdown — markdownlint-cli2 --fix, then prettier (pinned, see
#                PRETTIER_VERSION); options in .prettierrc.json (prose-wrap always,
#                print-width 160). Prettier owns markdown line width; MD013 is
#                disabled in .markdownlint.jsonc so the two never fight over it.
#
# NOTE: unlike lint.sh, this script runs NO checkers — no golangci-lint run, no
# go test, and none of shellcheck / yamllint / hadolint / checkmake. Those are
# checks, not formatters, and belong to lint.sh. `--fix` never fails on code problems;
# it only rewrites. `--check` fails (exit 1) purely to signal "these files are
# not formatted", so it can gate CI without mutating.
#
# ── usage spec (mirror of lint.sh's shared scope/mode flags) ──
# Hand-parsed at runtime (plain-bash script; no host `usage`). The scope/mode
# flags below are a strict subset of lint.sh's contract, byte-for-byte on the
# shared flags, so a caller can pass the same --all/--changed/--check/--fix.
#USAGE bin "fmt.sh"
#USAGE about "Single source of truth for repo formatting (all formatters run from pinned Docker images; no linters/tests)."
#USAGE flag "--all"     help="Format every tracked file (git ls-files) — the full pass. Default scope."
#USAGE flag "--changed" help="Format only this turn's changed files (unstaged + staged + new untracked + committed-this-turn)."
#USAGE flag "--check"   help="Report files that would be reformatted without modifying them; exit 1 if any. Default is to rewrite."
#USAGE flag "--fix"     help="Rewrite files in place (gofmt, golangci fmt, shfmt, markdownlint --fix, prettier), then report what changed. Default."
#USAGE flag "--porcelain" help="With --fix, print one NUL-terminated changed-file path to stdout instead of the human summary. For programmatic callers (lint.sh)."
set -uo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT" || exit 1

# ── Pinned formatter images. Kept in sync with lint.sh (which reads GO_IMAGE
# from the Makefile the same way). These MUST match lint.sh's copies so the fix
# pass lint.sh delegates here uses identical tooling. ───────────────────────────
GO_IMAGE="$(grep -m1 '^GO_IMAGE *:=' "$REPO_ROOT/Makefile" | sed 's/.*:= *//')"
LINT_IMAGE="golangci/golangci-lint:v2.12.2"
SHFMT_IMAGE="mvdan/shfmt:v3.13.1"
MARKDOWN_IMAGE="davidanson/markdownlint-cli2:v0.23.0"
PRETTIER_IMAGE="node:22-alpine"
# prettier itself is NOT baked into node:22-alpine; npx fetches it. Pin the exact
# version (not a floating range) so the same wrapping is produced on every host —
# otherwise a --check on one prettier release disagrees with a --fix on another.
PRETTIER_VERSION="3.9.5"

# ── Args ─────────────────────────────────────────────────────────────────────
# Mirrors lint.sh's parser for the shared flags. NOTE the default MODE here is
# --fix (a formatter's natural action is to rewrite), whereas lint.sh defaults to
# --check. Scope default is --all, matching lint.sh.
# PORCELAIN=1 emits NUL-terminated "<path>" records on stdout (for lint.sh) and
# suppresses the human summary — machine-readable and path-safe.
SCOPE="all" # all | changed
FIX=1       # 1 = --fix (rewrite), 0 = --check (report only)
PORCELAIN=0 # 1 = porcelain output (see note above)
while [[ $# -gt 0 ]]; do
  case "$1" in
    --all) SCOPE="all" ;;
    --changed) SCOPE="changed" ;;
    --check) FIX=0 ;;
    --fix) FIX=1 ;;
    --porcelain) PORCELAIN=1 ;;
    -h | --help)
      sed -n 's/^#USAGE flag "\([^"]*\)"[^=]*help="\(.*\)"$/  \1\t\2/p' "$0"
      exit 0
      ;;
    *)
      echo "fmt.sh: unknown flag '$1'" >&2
      exit 64
      ;;
  esac
  shift
done

# ── File selection ───────────────────────────────────────────────────────────
# Identical logic to lint.sh: populate per-type arrays for the chosen scope,
# restricting to files that still exist on disk so deletions never reach a
# formatter.
go_files=()
sh_files=()
md_files=()

# Markdown files exempt from formatting — LLM-facing resource docs whose exact
# layout is load-bearing (compact JSON, unwrapped tables). MUST stay in sync with
# lint.sh's md_format_exempt.
md_format_exempt() {
  local path="$1"
  case "$path" in
    internal/resources/docs/filter-dsl.md) return 0 ;;
    *) return 1 ;;
  esac
}

classify() {
  local f
  while IFS= read -r f; do
    [[ -n "$f" ]] || continue
    [[ -f "$f" ]] || continue
    case "$f" in
      *.go) go_files+=("$f") ;;
      *.sh) sh_files+=("$f") ;;
      *.md | *.mdx) md_format_exempt "$f" || md_files+=("$f") ;;
      *) ;;
    esac
  done
  return 0
}

# Same as lint.sh: union in files touched by commits made during this turn, which
# would otherwise escape --changed once committed. See lint.sh for the full
# rationale (.claude/last-prompt-ts, BASE..HEAD bound).
committed_this_turn() {
  local ts_file="$REPO_ROOT/.claude/last-prompt-ts" prompt_ts base_ref base range candidate mb
  [[ -r "$ts_file" ]] || return 0
  prompt_ts=$(cat "$ts_file" 2>/dev/null)
  case "$prompt_ts" in
    '' | *[!0-9]*) return 0 ;;
    *) ;;
  esac
  base_ref=$(git rev-parse --abbrev-ref '@{upstream}' 2>/dev/null)
  for candidate in "$base_ref" origin/main main; do
    [[ -n "$candidate" ]] || continue
    if mb=$(git merge-base "$candidate" HEAD 2>/dev/null); then
      base="$mb"
      break
    fi
  done
  range="${base:+$base..}HEAD"
  git log --no-merges --since="@$prompt_ts" --name-only --pretty=format: "$range" 2>/dev/null || true
}

if [[ "$SCOPE" = all ]]; then
  classify < <(git ls-files)
else
  classify < <({
    git diff --name-only
    git diff --cached --name-only
    git ls-files --others --exclude-standard
    committed_this_turn
  } | sort -u)
fi

# ── Formatters ───────────────────────────────────────────────────────────────
# In --fix each formatter rewrites in place and records changed files into
# `fixed_files`. In --check each reports files that WOULD change into `unformatted`
# without mutating, and the script exits 1 if any is non-empty.
fixed_files=""
unformatted=""
record_fixed() {
  local path="$1"
  fixed_files+="${path#./}"$'\n'
  return 0
}
record_unformatted() {
  local path="$1"
  unformatted+="${path#./}"$'\n'
  return 0
}
prepull() {
  local image="$1"
  docker pull -q "$image" >/dev/null 2>&1 || true
  return 0
}

# Content fingerprint of each still-existing path, so a before/after snapshot
# reveals which files a formatter rewrote (used for both --fix reporting and by
# lint.sh's delegation). Not a security context — just change detection — but we
# use SHA-256 so scanners don't flag a weak hash. Portable across Linux
# (`sha256sum`) and macOS (`shasum -a 256`): both print "<hash>  <path>" with two
# spaces. Anchor the sed to the hash→path separator only — a SHA-256 hash is 64
# hex chars, so `^<64hex><space><space>` → `<hash><space>` collapses just the
# separator, leaving any double-space *inside a path* intact (a plain `s/  / /`
# would eat the first double-space anywhere on the line and desync the snapshot).
# Callers below split the single-space line on `${line#* }`. Kept byte-for-byte
# in sync with lint.sh's file_hashes.
if command -v sha256sum >/dev/null 2>&1; then
  _hash_one() {
    local path="$1"
    sha256sum "$path" | sed -E 's/^([0-9a-f]{64})  /\1 /'
    return 0
  }
elif command -v shasum >/dev/null 2>&1; then
  _hash_one() {
    local path="$1"
    shasum -a 256 "$path" | sed -E 's/^([0-9a-f]{64})  /\1 /'
    return 0
  }
else
  # no hasher: change detection degrades to "nothing changed"
  _hash_one() { return 0; }
fi
file_hashes() {
  local f
  for f in "$@"; do [[ -f "$f" ]] && _hash_one "$f"; done 2>/dev/null || true
  return 0
}
# Emit the paths present in $after (hash list) but changed vs $before, via the
# recorder passed as $3.
report_changed() {
  local before="$1" after="$2" recorder="$3" line
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    grep -qxF "$line" <<<"$before" || "$recorder" "${line#* }"
  done <<<"$after"
  return 0
}

fmt_go() {
  [[ ${#go_files[@]} -gt 0 ]] || return 0
  prepull "$GO_IMAGE"
  prepull "$LINT_IMAGE"
  local lint_cache=(-v thehivemcp-gomod:/go/pkg/mod -v thehivemcp-gobuild:/root/.cache/go-build -v thehivemcp-golangci-cache:/root/.cache/golangci-lint)
  local before after
  local -a go_paths=()
  local f
  for f in "${go_files[@]}"; do go_paths+=("/app/$f"); done
  before=$(file_hashes "${go_files[@]}")

  if [[ "$FIX" -eq 1 ]]; then
    docker run -i --rm -v "$REPO_ROOT":/app -w /app "$GO_IMAGE" \
      gofmt -l -w "${go_paths[@]}" >/dev/null 2>&1 || true
    docker run -i --rm -v "$REPO_ROOT":/app "${lint_cache[@]}" -w /app "$LINT_IMAGE" \
      golangci-lint fmt ./... >/dev/null 2>&1 || true
    after=$(file_hashes "${go_files[@]}")
    report_changed "$before" "$after" record_fixed
  else
    local out
    if ! out=$(docker run -i --rm -v "$REPO_ROOT":/app -w /app "$GO_IMAGE" \
      gofmt -l "${go_paths[@]}" 2>&1) || [[ -n "$out" ]]; then
      while IFS= read -r f; do [[ -n "$f" ]] && record_unformatted "${f#/app/}"; done <<<"$out"
    fi
  fi
}

fmt_shell() {
  [[ ${#sh_files[@]} -gt 0 ]] || return 0
  prepull "$SHFMT_IMAGE"
  local out
  if [[ "$FIX" -eq 1 ]]; then
    out=$(docker run --rm -v "$REPO_ROOT":/mnt -w /mnt "$SHFMT_IMAGE" \
      -i 2 -ci -l -w "${sh_files[@]}" 2>&1)
    while IFS= read -r f; do [[ -n "$f" ]] && record_fixed "$f"; done <<<"$out"
  else
    out=$(docker run --rm -v "$REPO_ROOT":/mnt -w /mnt "$SHFMT_IMAGE" \
      -i 2 -ci -l "${sh_files[@]}" 2>&1)
    while IFS= read -r f; do [[ -n "$f" ]] && record_unformatted "$f"; done <<<"$out"
  fi
}

fmt_markdown() {
  [[ ${#md_files[@]} -gt 0 ]] || return 0
  prepull "$MARKDOWN_IMAGE"
  prepull "$PRETTIER_IMAGE"
  local before after
  local -a app_paths=()
  local f
  for f in "${md_files[@]}"; do app_paths+=("/app/$f"); done
  before=$(file_hashes "${md_files[@]}")

  if [[ "$FIX" -eq 1 ]]; then
    docker run -i --rm -v "$REPO_ROOT":/app -w /app "$MARKDOWN_IMAGE" \
      markdownlint-cli2 --fix "${app_paths[@]}" >/dev/null 2>&1 || true
    docker run -i --rm -v "$REPO_ROOT":/app -w /app "$PRETTIER_IMAGE" \
      npx --yes "prettier@$PRETTIER_VERSION" --write "${app_paths[@]}" >/dev/null 2>&1 || true
    after=$(file_hashes "${md_files[@]}")
    report_changed "$before" "$after" record_fixed
  else
    # Report-only: prettier --list-different exits 0 (all formatted), 1 (some would
    # be rewritten — file list on stdout), or ≥2 (prettier itself errored).
    # markdownlint violations are a *lint* concern (lint.sh), not formatting, so
    # --check here only flags prettier-level reformatting. Keep stdout (the file
    # list) separate from stderr (diagnostics, passed through) and honor the exit
    # code so a real prettier failure surfaces instead of masquerading as "clean".
    local out rc
    out=$(docker run -i --rm -v "$REPO_ROOT":/app -w /app "$PRETTIER_IMAGE" \
      npx --yes "prettier@$PRETTIER_VERSION" --list-different "${app_paths[@]}")
    rc=$?
    if [[ "$rc" -ge 2 ]]; then
      echo "fmt.sh: prettier check failed (exit $rc)" >&2
      exit "$rc"
    fi
    while IFS= read -r f; do [[ -n "$f" ]] && record_unformatted "${f#/app/}"; done <<<"$out"
  fi
}

# ── Dispatch ──────────────────────────────────────────────────────────────────
fmt_go
fmt_shell
fmt_markdown

# ── Report ───────────────────────────────────────────────────────────────────
summary() {
  local parts=()
  [[ "${#go_files[@]}" -gt 0 ]] && parts+=("${#go_files[@]} go")
  [[ "${#sh_files[@]}" -gt 0 ]] && parts+=("${#sh_files[@]} sh")
  [[ "${#md_files[@]}" -gt 0 ]] && parts+=("${#md_files[@]} md")
  if [[ "${#parts[@]}" -eq 0 ]]; then
    printf 'no files'
    return 0
  fi
  local out="${parts[0]}" i
  for i in "${parts[@]:1}"; do out+=", $i"; done
  printf '%s' "$out"
  return 0
}

list_block() {
  local label="$1" list="$2" uniq
  uniq=$(printf '%s' "$list" | grep -v '^$' | sort -u)
  [[ -n "$uniq" ]] || return 0
  printf '\n%s:\n%s' "$label" "$(printf '%s' "$uniq" | sed 's/^/  /')"
  return 0
}

if [[ "$PORCELAIN" -eq 1 ]]; then
  # Machine-readable output for programmatic callers (lint.sh): one NUL-terminated
  # changed-file path per record, deduped, nothing else on stdout. NUL-termination
  # keeps it robust against any path (spaces, colons, leading whitespace), unlike
  # scraping the human "Formatted:" block. --porcelain implies --fix.
  printf '%s' "$fixed_files" | grep -v '^$' | sort -u | tr '\n' '\0'
  exit 0
fi

if [[ "$FIX" -eq 1 ]]; then
  # summary() counts files CONSIDERED; fixed_files is what actually changed. Only
  # claim work was done when something was reformatted, so a no-op re-run reads as
  # "already formatted" rather than a misleading "fmt done (N sh)".
  n_fixed=$(printf '%s' "$fixed_files" | grep -vc '^$')
  if [[ "$n_fixed" -gt 0 ]]; then
    printf '✓ formatted %d of %s%s\n' "$n_fixed" "$(summary)" "$(list_block 'Formatted' "$fixed_files")"
  else
    printf '✓ already formatted (%s)\n' "$(summary)"
  fi
  exit 0
fi

if [[ -z "$unformatted" ]]; then
  printf '✓ fmt check passed (%s)\n' "$(summary)"
  exit 0
fi
printf '✗ fmt check: files need formatting (run with --fix):%s\n' \
  "$(list_block 'Unformatted' "$unformatted")" >&2
exit 1
