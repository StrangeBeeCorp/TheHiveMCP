# TheHiveMCP for Claude Code

@AGENTS.md

## Claude Code

- Load the `develop-feature` skill (`.claude/skills/develop-feature/`) before changing code: where things live, the rules for model-facing code, and what a pull
  request must update.
- Run `make check-fast` before you stop: the gates your changed paths select, listed in `docs/reference/gates.md`.
- Per-machine notes go in `CLAUDE.local.md` and per-machine settings in `settings.local.json` next to `settings.json`. Both stay out of git; the public-safety
  gate refuses them.
