# TheHiveMCP for Claude Code

@AGENTS.md

## Claude Code

- Load the `develop-feature` skill (`.claude/skills/develop-feature/`) before changing code: where things live, the rules for model-facing code, and what a pull
  request must update.
- The hooks of `.claude/settings.json` send every tool call to `bin/harness hook`, which allows or denies with what to do instead (`internal/harness/policy/`).
  Ship with `make pr` (the pr gates at HEAD, recorded, then the push), never to main or the public mirror; a person merges. From a Claude session the push needs
  that evidence and a ready review of the same HEAD; only a person may pass `--person` to push without them. The hooks read shell commands, not the code a main
  session may run (`python3`): CI runs every gate again on the pull request, and the person who merges is the last check. Every stop runs `make check-fast` (the
  gates your changed paths select, listed in `docs/reference/gates.md`): an agent does not stop red, and after six red rounds it writes
  `.cache/harness/BLOCKED.md`.
- Implement test first with the `implement-task` workflow: the Tester (`.claude/agents/tester.md`) writes the tests, the Coder (`.claude/agents/coder.md`) the
  code, and neither may edit the other's files.
- Per-machine notes go in `CLAUDE.local.md` and per-machine settings in `settings.local.json` next to `settings.json`. Both stay out of git; the public-safety
  gate refuses them.
