#!/usr/bin/env bash
# Single source of truth for repo lint/format CHECKS. All check tools run from
# pinned Docker images — no local toolchain required (this is a Go repo with no
# host Go install; see the Makefile's install-dev-deps).
#
# FORMATTING lives in a sibling script, scripts/fmt.sh (gofmt, golangci fmt,
# shfmt, markdownlint --fix, prettier). In --fix mode this script delegates the
# mutation pass to fmt.sh (see run_fmt) and then layers the read-only checkers on
# top; the observable CLI contract is unchanged (--fix still "auto-fixes
# cosmetics + safe lint fixes"). `golangci-lint run --fix` (a LINT fix, not a
# formatter) stays here.
#
# The CLI is defined by a `usage` (usage.jdx.dev) contract, OpenAPI-style. The
# canonical spec lives in the sb-thehive-mcp PLUGIN (its lint.usage.kdl), NOT in
# this repo — and drives `usage lint`, doc generation, and shell completions.
# The #USAGE comments in this file MIRROR that plugin contract. The plugin's
# lint-script-setup skill enforces the mirror: it runs `usage` FROM A PINNED
# DOCKER IMAGE (the plugin's Dockerfile.usage — no host-installed `usage`) to
# (a) validate the plugin .kdl and (b) fail if the plugin .kdl and these #USAGE
# comments have drifted apart. Because we can't assume host `usage`, this script
# runs under a plain `bash` shebang and parses its own args — the #USAGE
# comments are inert at runtime (read only by that offline drift check), so the
# arg loop below is the actual runtime parser. The plugin .kdl leads; mirror any
# flag change from it into both the #USAGE block and that loop.
#
# Consumers call this script:
#   • `make lint`             → --all --check         (whole repo, no mutation)
#   • `make lint-changed`     → --changed --check      (working-tree files only)
#   • `make lint-fix`         → --all --fix            (whole repo, auto-fix)
#   • `make lint-fix-changed` → --changed --fix        (working-tree, auto-fix)
#   • a Stop hook             → --changed --fix --hook  (auto-fix + JSON below)
#
# In --hook mode the script speaks the Stop-hook JSON contract on exit 0:
#   - No blocking issue (clean stop) → {systemMessage (for the user)} ONLY. The
#     Stop event is respected: systemMessage is display-only and does not
#     continue the turn. additionalContext is NOT emitted here — a Stop hook's
#     additionalContext would continue the turn, which is exactly the bug we
#     avoid. Any stale-chunk re-read ranges are instead persisted to
#     .claude/lint-pending-rereads for the companion UserPromptSubmit hook to
#     inject on the next prompt (when the user resumes the session).
#   - At least one blocking issue → {decision:"block", reason} folding the
#     failures, the auto-fixed list, and the stale-chunk ranges together, so the
#     agent keeps going and sees everything it must fix and re-read. A block
#     already keeps the turn running, so the ranges ride the reason inline and
#     no state file is written.
#
# ── usage spec (mirror of the PLUGIN's lint.usage.kdl — the canonical contract) ──
# These #USAGE comments are NOT parsed at runtime (plain-bash script; see the
# shebang note above). They exist only so the plugin's drift check —
# `usage generate json -f scripts/lint.sh` vs the plugin .kdl, run from Docker
# by the lint-script-setup skill's verify-setup.sh — normalizes this file to the
# same JSON as the plugin .kdl and fails on any divergence.
#
# NOTE: every #USAGE line below MUST be a byte-for-byte copy of the matching
# line in the PLUGIN's lint.usage.kdl (minus the leading "#USAGE "). Do not edit
# the help text here; edit the plugin .kdl first, then re-run the skill.
#USAGE bin "lint.sh"
#USAGE about "Single source of truth for repo lint/format checks (all tools run from pinned Docker images)."
#USAGE flag "--all"         help="Check every tracked file (git ls-files) — the full gate. Default scope."
#USAGE flag "--changed"     help="Check only this turn's changed files (unstaged + staged + new untracked + committed-this-turn)."
#USAGE flag "--check"       help="Report problems without modifying files. Default."
#USAGE flag "--fix"         help="Auto-fix cosmetics (ruff format, shfmt) and ruff's SAFE lint fixes in place, then report loudly."
#USAGE flag "--hook"        help="Emit the Stop-hook JSON contract (exit 0): a clean stop returns {systemMessage} only (the Stop event is respected — no additionalContext, which would continue the turn), persisting any re-read line ranges to .claude/lint-pending-rereads for the UserPromptSubmit hook to inject next prompt; a blocking issue returns {decision:\"block\", reason} folding failures, auto-fixed files, and those ranges together."
#USAGE flag "--only-custom" help="Run only the checks the sb-thehive-mcp plugin Stop hook doesn't. Example: mypy and compose-overlay validation."
set -uo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT" || exit 1

# ── Pinned check tools. This script is the source of truth for the repo's check
# versions (the plugin's Stop hook just launches it). GO_IMAGE is read from the
# Makefile so `go test`/`go build` here never drift from `make test`. LINT_IMAGE
# and the rest are pinned right here — this script is their single reference.
# Only list the images this repo's file types need.
GO_IMAGE="$(grep -m1 '^GO_IMAGE *:=' "$REPO_ROOT/Makefile" | sed 's/.*:= *//')"
LINT_IMAGE="golangci/golangci-lint:v2.12.2"
SHELLCHECK_IMAGE="koalaman/shellcheck:v0.11.0"
SHFMT_IMAGE="mvdan/shfmt:v3.13.1"
MARKDOWN_IMAGE="davidanson/markdownlint-cli2:v0.23.0"
LYCHEE_IMAGE="lycheeverse/lychee:0.24.2"
# prettier (markdown prose-wrap) moved to scripts/fmt.sh — the formatter source
# of truth. lint.sh only runs read-only checks now, so no PRETTIER_IMAGE here.
YAMLLINT_IMAGE="cytopia/yamllint@sha256:3e9eb827ab2b12a5ea5f49d4257bb3aca94bba9f1ba427c8bc7f2456385a5204"
HADOLINT_IMAGE="hadolint/hadolint:v2.14.0"
CHECKMAKE_IMAGE="cytopia/checkmake@sha256:23116ee551144f1021b294d3ede266ecb760272e0d6f2833f8a3d38b81beffb8"
YL_RULES='{extends: relaxed, rules: {line-length: disable, document-start: disable, comments: disable, comments-indentation: disable, empty-lines: disable, trailing-spaces: disable}}'

# ── Args ─────────────────────────────────────────────────────────────────────
# Hand-parsed (no host `usage` at runtime). The flags, their help, and the
# defaults documented in the #USAGE block above mirror the plugin's
# lint.usage.kdl — the contract. Change the plugin .kdl first, then this.
SCOPE="all" # all | changed
FIX=0       # 0 = --check, 1 = --fix
HOOK=0      # 0 = text output, 1 = Stop-hook JSON contract
ONLY_CUSTOM=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --all) SCOPE="all" ;;
    --changed) SCOPE="changed" ;;
    --check) FIX=0 ;;
    --fix) FIX=1 ;;
    --hook) HOOK=1 ;;
    --only-custom) ONLY_CUSTOM=1 ;;
    -h | --help)
      sed -n 's/^#USAGE flag "\([^"]*\)"[^=]*help="\(.*\)"$/  \1\t\2/p' "$0"
      exit 0
      ;;
    *)
      echo "lint.sh: unknown flag '$1'" >&2
      exit 64
      ;;
  esac
  shift
done

# ── File selection ───────────────────────────────────────────────────────────
# Populate the *_files arrays for the chosen scope. Both scopes restrict to
# files that still exist on disk so deletions never reach a checker.
go_files=()
sh_files=()
md_files=()
yaml_files=()
dockerfiles=()
mk_files=()

# Markdown files exempt from lint/format. These are LLM-facing resource docs
# whose exact layout is load-bearing — compact single-line JSON and unwrapped
# table rows are what the model is shown — so markdownlint (--fix) and prettier
# (--prose-wrap) must not rewrite them. Paths are handed to the tools
# explicitly, so the tools' own ignore files never see them; skip here instead.
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
      *.yaml | *.yml) yaml_files+=("$f") ;;
      *.mk) mk_files+=("$f") ;;
      *) ;;
    esac
    case "$(basename "$f")" in
      Dockerfile | Dockerfile.*) dockerfiles+=("$f") ;;
      GNUmakefile | Makefile | makefile | Makefile.*) mk_files+=("$f") ;;
      *) ;;
    esac
  done
  return 0
}

# Files touched by commits made *during this turn* would otherwise escape the
# --changed scope: once committed, `git diff HEAD` and untracked detection
# report nothing. The plugin's UserPromptSubmit hook stamps the turn start in
# .claude/last-prompt-ts; reuse it (we own no UserPromptSubmit hook) to union in
# files touched by commits made at/after that second. The walk is bounded to
# this branch's own commits (BASE..HEAD) so a date filter alone can't sweep in
# base commits others authored. BASE = merge-base(HEAD, upstream|origin/main).
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

# ── Pre-fix snapshot (Stop-hook mode only) ─────────────────────────────────────
# In `--fix --hook` we record the exact NEW-file line ranges each fixer rewrote,
# so the next prompt can tell the agent which lines to re-read (its in-context
# copy is now stale). We can't know in advance which files a fixer will touch,
# so snapshot EVERY candidate up front — before any fixer runs — into a tempdir,
# then diff each against its post-fix on-disk version in the report section.
# Only spent when both HOOK and FIX are set; a plain `make lint-fix` at the
# terminal pays nothing for it.
SNAPSHOT_DIR=""
# All candidate paths across every file type, deduped — reused for the snapshot
# and, in the report section, for the before/after diff. Built from the arrays
# with the +() guard so an empty array is safe under `set -u`.
all_candidates() {
  {
    [[ ${#go_files[@]} -gt 0 ]] && printf '%s\n' "${go_files[@]}"
    [[ ${#sh_files[@]} -gt 0 ]] && printf '%s\n' "${sh_files[@]}"
    [[ ${#md_files[@]} -gt 0 ]] && printf '%s\n' "${md_files[@]}"
    [[ ${#yaml_files[@]} -gt 0 ]] && printf '%s\n' "${yaml_files[@]}"
    [[ ${#dockerfiles[@]} -gt 0 ]] && printf '%s\n' "${dockerfiles[@]}"
    [[ ${#mk_files[@]} -gt 0 ]] && printf '%s\n' "${mk_files[@]}"
    # In --fix mode check_go runs `golangci-lint run --fix ./...` over the WHOLE
    # module, which can rewrite any .go file (not just the changed scope). Snapshot
    # them all so an out-of-scope safe-fix gets a proper re-read range, not just a
    # bare filename in the "Auto-fixed (lint)" list.
    [[ "$FIX" -eq 1 ]] && git ls-files '*.go'
  } | sort -u
  return 0
}
if [[ "$HOOK" -eq 1 ]] && [[ "$FIX" -eq 1 ]]; then
  SNAPSHOT_DIR=$(mktemp -d)
  trap 'rm -rf "$SNAPSHOT_DIR"' EXIT
  while IFS= read -r f; do
    [[ -n "$f" ]] && [[ -f "$f" ]] || continue
    mkdir -p "$SNAPSHOT_DIR/$(dirname "$f")"
    cp "$f" "$SNAPSHOT_DIR/$f"
  done < <(all_candidates)
fi

# ── Checkers ───────────────────────────────────────────────────────────────
# Each appends a "=== name ===\n<output>\n" block to `failures` on a blocking
# failure. Auto-fixes are reported loudly but never populate `failures`.
failures=""
fixed_files=""
lint_fixed_files=""

record_fixed() {
  local path="$1"
  fixed_files+="${path#./}"$'\n'
  return 0
}
record_lint_fixed() {
  local path="$1"
  lint_fixed_files+="${path#./}"$'\n'
  return 0
}
prepull() {
  local image="$1"
  docker pull -q "$image" >/dev/null 2>&1 || true
  return 0
}

# ── Formatting delegation ──────────────────────────────────────────────────────
# fmt.sh is the single source of truth for FORMATTING (gofmt, golangci fmt,
# shfmt, markdownlint --fix, prettier). In --fix mode we invoke it once, for the
# same scope, and fold the files it rewrote into `fixed_files` so the Stop-hook
# re-read machinery and the "Auto-fixed" report see them. The checkers below stay
# here; only mutation moved. `golangci-lint run --fix` is a LINT fix, not a
# formatter, so it remains in check_go (tracked as lint_fixed_files).
FMT_SCRIPT="$REPO_ROOT/scripts/fmt.sh"
run_fmt() {
  [[ "$FIX" -eq 1 ]] || return 0
  local f
  # fmt.sh --porcelain emits the files it rewrote as NUL-terminated records on
  # stdout (machine-readable, path-safe — no scraping the human "Formatted:"
  # block). Diagnostics go to stderr, which we let through. It never fails in
  # --fix mode. Read via process substitution (not a pipe) so record_fixed
  # mutates fixed_files in THIS shell.
  while IFS= read -r -d '' f; do
    [[ -n "$f" ]] && record_fixed "$f"
  done < <("$FMT_SCRIPT" "--$SCOPE" --fix --porcelain 2>/dev/null || true)
  return 0
}

# Content fingerprint of each still-existing path so a caller can diff a
# before/after snapshot to learn which files a tool rewrote in place. Not a
# security context — just change detection — but we use SHA-256 so scanners don't
# flag a weak hash. Portable across Linux (`sha256sum`) and macOS (`shasum -a
# 256`): both print "<hash>  <path>" with two spaces. Anchor the sed to the
# hash→path separator only — a SHA-256 hash is 64 hex chars, so
# `^<64hex><space><space>` → `<hash><space>` collapses just the separator,
# leaving any double-space *inside a path* intact (a plain `s/  / /` would eat
# the first double-space anywhere on the line and desync the snapshot). Callers
# split the single-space line on `${line#* }`. Kept byte-for-byte in sync with
# fmt.sh's file_hashes.
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
record_changed() {
  local before="$1" after="$2" line
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    grep -qxF "$line" <<<"$before" || record_lint_fixed "${line#* }"
  done <<<"$after"
  return 0
}

# ── Go (golangci-lint + fast tests). Skipped under --only-custom: the plugin's
# Stop hook already runs all of these, so a local hook delegates them.
# FORMATTING (gofmt -w, golangci-lint fmt) is delegated to fmt.sh via run_fmt();
# what stays here is the LINT auto-fix (`golangci-lint run --fix`, a lint fix not
# a formatter), the check-only gofmt -l gate, the blocking `golangci-lint run`,
# and `go test -short`. The repo ships its own .golangci.yml, so golangci-lint
# auto-discovers it (no -c). Cache mounts are the Makefile's named volumes
# (linux-native, populated in-container) so the script works identically on host
# and docker-in-docker. ─────────────────────────────────────────────────────────
check_go() {
  [[ ${#go_files[@]} -gt 0 ]] || return 0
  prepull "$GO_IMAGE"
  prepull "$LINT_IMAGE"
  local cache=(-v thehivemcp-gomod:/go/pkg/mod -v thehivemcp-gobuild:/root/.cache/go-build)
  local lint_cache=("${cache[@]}" -v thehivemcp-golangci-cache:/root/.cache/golangci-lint)
  local out
  local -a go_paths=()
  local f
  for f in "${go_files[@]}"; do go_paths+=("/app/$f"); done

  if [[ "$FIX" -eq 1 ]]; then
    # Formatting already ran via run_fmt(). Apply golangci's SAFE lint fixes and
    # record them separately (lint_fixed_files), keeping the "lint fix" vs
    # "format" distinction the Stop-hook report relies on.
    #
    # `golangci-lint run --fix ./...` operates on the WHOLE module, so it can
    # rewrite .go files outside the changed scope. Snapshot EVERY tracked .go file
    # (not just go_files) before/after, so an out-of-scope safe-fix is reported and
    # — in Stop-hook mode — gets a re-read range instead of silently mutating a
    # file whose in-context copy then goes stale. The hash pass over all .go files
    # is cheap next to the golangci/docker run itself.
    local before after
    local -a all_go=()
    while IFS= read -r f; do [[ -n "$f" ]] && all_go+=("$f"); done < <(git ls-files '*.go')
    before=$(file_hashes "${all_go[@]}")
    docker run -i --rm -v "$REPO_ROOT":/app "${lint_cache[@]}" -w /app "$LINT_IMAGE" \
      golangci-lint run --fix ./... >/dev/null 2>&1 || true
    after=$(file_hashes "${all_go[@]}")
    record_changed "$before" "$after"
  else
    if ! out=$(docker run -i --rm -v "$REPO_ROOT":/app -w /app "$GO_IMAGE" \
      gofmt -l "${go_paths[@]}" 2>&1) || [[ -n "$out" ]]; then
      failures+="=== gofmt -l (needs formatting) ===
$out

"
    fi
  fi

  # Blocking lint gate over the whole module (cross-file findings still apply).
  if ! out=$(docker run -i --rm -v "$REPO_ROOT":/app "${lint_cache[@]}" -w /app "$LINT_IMAGE" \
    golangci-lint run ./... 2>&1); then
    failures+="=== golangci-lint ===
$out

"
  fi

  # Fast unit tests (mirrors `make test`: -short skips the integration suite, so
  # results are go-test-cached and reruns are near-instant).
  if ! out=$(docker run -i --rm -v "$REPO_ROOT":/app -w /app "${cache[@]}" "$GO_IMAGE" \
    go test -short ./... 2>&1); then
    failures+="=== go test -short ===
$out

"
  fi
  return 0
}

# Shell: shellcheck (lint) always runs here. shfmt FORMATTING is delegated to
# fmt.sh via run_fmt() in --fix mode; in --check mode we still report files that
# would be reformatted (shfmt -l) as a blocking failure.
check_shell() {
  [[ ${#sh_files[@]} -gt 0 ]] || return 0
  prepull "$SHELLCHECK_IMAGE"
  prepull "$SHFMT_IMAGE"
  local out
  if ! out=$(docker run --rm -v "$REPO_ROOT":/mnt -w /mnt "$SHELLCHECK_IMAGE" \
    "${sh_files[@]}" 2>&1); then
    failures+="=== shellcheck ===
$out

"
  fi
  if [[ "$FIX" -eq 0 ]]; then
    # Report-only: list the files that WOULD be reformatted (shfmt -l) rather
    # than dumping the full per-file diff (shfmt -d), which over the whole-repo
    # --all scope is a wall of output. Run `make lint-fix` / `--fix` to apply.
    if out=$(docker run --rm -v "$REPO_ROOT":/mnt -w /mnt "$SHFMT_IMAGE" \
      -i 2 -ci -l "${sh_files[@]}" 2>&1) && [[ -z "$out" ]]; then
      : # all formatted
    else
      failures+="=== shfmt (needs formatting; run with --fix) ===
$out

"
    fi
  fi
  return 0
}

# ── Markdown: markdownlint-cli2 (lint) + lychee link check. FORMATTING
# (markdownlint --fix, prettier prose-wrap) is delegated to fmt.sh via run_fmt();
# what stays here is the read-only markdownlint gate (--check mode) and the
# lychee --offline link check (local refs only, so a remote outage can't fail
# us), which always runs. The repo ships its own .markdownlint.jsonc,
# auto-discovered under -w /app. ────────────────────────────────────────────────
check_markdown() {
  [[ ${#md_files[@]} -gt 0 ]] || return 0
  prepull "$MARKDOWN_IMAGE"
  prepull "$LYCHEE_IMAGE"
  local out
  local -a app_paths=()
  local f
  for f in "${md_files[@]}"; do app_paths+=("/app/$f"); done

  if [[ "$FIX" -eq 0 ]] && ! out=$(docker run -i --rm -v "$REPO_ROOT":/app -w /app "$MARKDOWN_IMAGE" \
    markdownlint-cli2 "${app_paths[@]}" 2>&1); then
    failures+="=== markdownlint-cli2 ===
$out

"
  fi

  if ! out=$(docker run --init -i --rm -v "$REPO_ROOT":/input -w /input "$LYCHEE_IMAGE" \
    --offline "${md_files[@]}" 2>&1); then
    failures+="=== lychee ===
$out

"
  fi
  return 0
}

check_yaml() {
  [[ ${#yaml_files[@]} -gt 0 ]] || return 0
  prepull "$YAMLLINT_IMAGE"
  local out
  if ! out=$(docker run --rm -v "$REPO_ROOT":/data -w /data "$YAMLLINT_IMAGE" \
    -d "$YL_RULES" "${yaml_files[@]}" 2>&1); then
    failures+="=== yamllint ===
$out

"
  fi
  return 0
}

check_dockerfiles() {
  [[ ${#dockerfiles[@]} -gt 0 ]] || return 0
  prepull "$HADOLINT_IMAGE"
  local df out cfg=()
  [[ -f "$REPO_ROOT/.hadolint.yaml" ]] &&
    cfg=(-v "$REPO_ROOT/.hadolint.yaml":/.hadolint.yaml "$HADOLINT_IMAGE" hadolint --config /.hadolint.yaml)
  for df in "${dockerfiles[@]}"; do
    if [[ ${#cfg[@]} -gt 0 ]]; then
      out=$(docker run --rm -i "${cfg[@]}" - <"$df" 2>&1) || {
        failures+="=== hadolint ($df) ===
$out

"
      }
    else
      out=$(docker run --rm -i "$HADOLINT_IMAGE" hadolint - <"$df" 2>&1) || {
        failures+="=== hadolint ($df) ===
$out

"
      }
    fi
  done
  return 0
}

# ── Makefiles (checkmake). Reads its rule config from the repo's checkmake.ini,
# mounted read-only. Mirrors the plugin hook's checkmake step. ──────────────────
check_makefiles() {
  [[ ${#mk_files[@]} -gt 0 ]] || return 0
  prepull "$CHECKMAKE_IMAGE"
  local out cfg=() flag=()
  if [[ -f "$REPO_ROOT/checkmake.ini" ]]; then
    cfg=(-v "$REPO_ROOT/checkmake.ini":/checkmake.ini:ro)
    flag=(--config=/checkmake.ini)
  fi
  local -a app_paths=()
  local f
  for f in "${mk_files[@]}"; do app_paths+=("/app/$f"); done
  if ! out=$(docker run --rm -v "$REPO_ROOT":/app "${cfg[@]}" -w /app "$CHECKMAKE_IMAGE" \
    "${flag[@]}" "${app_paths[@]}" 2>&1); then
    failures+="=== checkmake ===
$out

"
  fi
  return 0
}

# ── REPO-SPECIFIC CUSTOM CHECKS ───────────────────────────────────────────────
# --only-custom exists so a repo's OWN local Stop hook can add checks the plugin
# lacks WITHOUT re-running the plugin's tools. This repo has NO such checks — the
# plugin Stop hook already covers every file type it contains (Go, shell,
# markdown, YAML, Dockerfiles, Makefile) — so check_custom is intentionally
# empty and --only-custom is a no-op. Add repo-only checks here if that changes.
check_custom() {
  # no checks beyond the plugin's; --only-custom does nothing (correct).
  return 0
}

# ── Dispatch ──────────────────────────────────────────────────────────────────
# --only-custom runs ONLY check_custom. Otherwise: in --fix mode do the FORMATTING
# pass first (run_fmt → fmt.sh), then run the full plugin-mirroring checker set.
# run_fmt is a no-op in --check mode. It runs AFTER the Stop-hook snapshot above
# (so its rewrites are captured for re-read ranges) and BEFORE the checkers (so a
# gofmt/shfmt problem is fixed, not re-reported).
if [[ "$ONLY_CUSTOM" -eq 1 ]]; then
  check_custom
else
  run_fmt
  check_go
  check_shell
  check_markdown
  check_yaml
  check_dockerfiles
  check_makefiles
  check_custom
fi

# ── Report ───────────────────────────────────────────────────────────────────
summary() {
  local parts=()
  [[ "${#go_files[@]}" -gt 0 ]] && parts+=("${#go_files[@]} go")
  [[ "${#sh_files[@]}" -gt 0 ]] && parts+=("${#sh_files[@]} sh")
  [[ "${#md_files[@]}" -gt 0 ]] && parts+=("${#md_files[@]} md")
  [[ "${#yaml_files[@]}" -gt 0 ]] && parts+=("${#yaml_files[@]} yaml")
  [[ "${#dockerfiles[@]}" -gt 0 ]] && parts+=("${#dockerfiles[@]} Dockerfile")
  [[ "${#mk_files[@]}" -gt 0 ]] && parts+=("${#mk_files[@]} Makefile")
  if [[ "${#parts[@]}" -eq 0 ]]; then
    printf 'no changed files'
    return 0
  fi
  local out="${parts[0]}" i
  for i in "${parts[@]:1}"; do out+=", $i"; done
  printf '%s' "$out"
  return 0
}

fixed_block() {
  local label="$1" list="$2" uniq
  uniq=$(printf '%s' "$list" | grep -v '^$' | sort -u)
  [[ -n "$uniq" ]] || return 0
  printf '\n%s:\n%s' "$label" "$(printf '%s' "$uniq" | sed 's/^/  /')"
  return 0
}
fixed="$(fixed_block 'Auto-fixed' "$fixed_files")$(fixed_block 'Auto-fixed (lint)' "$lint_fixed_files")"

# ── Auto-fixed line ranges (Stop-hook mode only) ───────────────────────────────
# For each file a fixer actually rewrote, diff its pre-fix snapshot against the
# current on-disk content and emit the NEW-file line ranges of each hunk — the
# lines the agent must re-read (its in-context copy is now stale). Collected into
# `ranges` and surfaced through the --hook JSON below: on a clean stop they are
# persisted to .claude/lint-pending-rereads for the UserPromptSubmit hook to
# inject next prompt (Stop must not emit additionalContext — it would continue
# the turn); on a blocking failure they are folded into the block `reason`
# inline (a block already keeps the turn running). Guarded on SNAPSHOT_DIR so it only runs in
# `--fix --hook`. Multiple non-adjacent hunks in one file → a comma-joined list.
ranges=""
if [[ -n "$SNAPSHOT_DIR" ]] && { [[ -n "$fixed_files" ]] || [[ -n "$lint_fixed_files" ]]; }; then
  fixed_rel=$(printf '%s%s' "$fixed_files" "$lint_fixed_files" | grep -v '^$' | sort -u)
  while IFS= read -r rel; do
    [[ -n "$rel" ]] || continue
    [[ -f "$rel" ]] && [[ -f "$SNAPSHOT_DIR/$rel" ]] || continue
    # Parse each hunk header "@@ -a,b +c,d @@": the new-file side is +c,d, where
    # d defaults to 1 when omitted. Emit "c-(c+d-1)" (single line -> "N-N").
    file_ranges=$(diff -u "$SNAPSHOT_DIR/$rel" "$rel" 2>/dev/null |
      grep '^@@' |
      sed -E 's/^@@ -[0-9]+(,[0-9]+)? \+([0-9]+)(,([0-9]+))? @@.*/\2 \4/' |
      while read -r start len; do
        len="${len:-1}"
        echo "$start-$((start + len - 1))"
      done |
      paste -sd, - | sed 's/,/, /g')
    [[ -n "$file_ranges" ]] && ranges="${ranges}  ${rel}: ${file_ranges}"$'\n'
  done <<<"$fixed_rel"
fi
# The re-read notice, prefixed to the ranges block. Empty when nothing was fixed.
ranges_block=""
[[ -n "$ranges" ]] && ranges_block="

Auto-fixed chunks (your in-context copy is stale — re-read before editing):
${ranges%$'\n'}"

if [[ "$HOOK" -eq 1 ]]; then
  if [[ -z "$failures" ]]; then
    # Clean stop: RESPECT the Stop event. systemMessage is display-only (does not
    # continue the turn); additionalContext WOULD continue it, so we must not emit
    # it here. Persist any stale-chunk ranges for the UserPromptSubmit hook to
    # inject on the next prompt.
    if [[ -n "$ranges_block" ]]; then
      mkdir -p "$REPO_ROOT/.claude"
      printf '%s\n' "${ranges_block#$'\n'}" >"$REPO_ROOT/.claude/lint-pending-rereads"
    fi
    jq -cn --arg msg "✓ Static analysis passed ($(summary))$fixed" '{systemMessage: $msg}'
    exit 0
  fi
  # At least one blocking issue: prevent the stop and hand the agent everything
  # it must act on — the failures, the auto-fixed file list, and the stale-chunk
  # ranges to re-read — via decision:block + reason (processed only on exit 0).
  jq -cn --arg reason "Lint checks failed — fix these before stopping:$fixed$ranges_block

$failures" \
    '{decision: "block", reason: $reason}'
  exit 0
fi

if [[ -z "$failures" ]]; then
  printf '✓ lint passed (%s)%s\n' "$(summary)" "$fixed"
  exit 0
fi
printf '✗ lint found problems:%s\n\n%s\n' "$fixed" "$failures" >&2
exit 1
