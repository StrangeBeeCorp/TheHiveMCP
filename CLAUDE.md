# TheHiveMCP for Claude Code

@AGENTS.md

## Claude Code

- Load the `develop-feature` skill (`.claude/skills/develop-feature/`) before changing code: where things live, the rules for model-facing code, and what a pull
  request must update.
- Ship with `make pr` (the pr gates at HEAD, recorded, then the push of this branch), never to main or the public mirror; a person merges. A Claude session
  never runs `git push`, and never merges, approves or updates the branch of a pull request: `bin/harness push`, which trusts a recorded verdict, and
  `--person`, which skips the review, are a person's. The permission rules of `.claude/settings.json` refuse their plain forms; CI runs every gate again on the
  pull request, and the person who merges is the last check.
- The hooks of `.claude/settings.json` send every tool call to `bin/harness hook` (`internal/harness/hooks/`, `internal/harness/policy/`): an edit or a read is
  judged by the role's paths, a command only for an environment file it names, and when an agent stops, every file it changed is judged by its role again,
  whatever command changed it. Every stop runs `make check-fast` (the gates your changed paths select, listed in `docs/reference/gates.md`): an agent does not
  stop red, and after six red rounds it writes `.cache/harness/BLOCKED.md`.
- Implement test first with the `implement-task` workflow: the Tester (`.claude/agents/tester.md`) writes the tests, the Coder (`.claude/agents/coder.md`) the
  code, and neither may edit the other's files.
- Per-machine notes go in `CLAUDE.local.md` and per-machine settings in `settings.local.json` next to `settings.json`. Both stay out of git; the public-safety
  gate refuses them.
