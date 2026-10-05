# ADR-0004 - The harness judges what changed, not the commands that changed it

- Status: accepted
- Date: October 5, 2026
- Deciders: TheHiveMCP maintainers
- Related: DL-8041 (the agent harness), DL-8086, [#205](https://github.com/StrangeBee/TheHiveMCP/pull/205)

## Context and problem statement

Coding agents write most of this repository's changes. The harness keeps them to the same rules a person follows: one registry of gates (`gates/registry.yaml`),
run at every step from the first edit to CI, and roles that keep the Tester to tests and the Coder to code.

Its first version also read every shell command an agent ran, through a bash parser, and refused the risky ones: a push, a write to the harness's records, a
change to a test by the Coder. A command can be spelt in endless ways. Six adversarial reviews of the shipping story (DL-8045) each found new spellings past the
parser: 25 of the last 38 fixes, and all 11 high findings. Meanwhile it refused ordinary work for reasons an agent could not guess.

## Decision

The harness judges outcomes, and leaves commands to Claude Code:

- **What changed.** When an agent stops, every file it changed since its first tool call is judged by its role, whatever command changed it. A change the role
  may not make is sent back with the command that restores the file. Edits and reads made through the editor are still judged by path, before they happen.
- **What only a person does.** Pushing, merging and approving are refused by Claude Code's permission rules (`.claude/settings.json`) in the forms Claude
  writes, and `make pr` is the one way a Claude session ships. Auto mode's classifier judges whether a command is destructive.
- **The last check.** CI runs every gate again on the pull request, and a person merges it.

## Consequences

- An agent runs the commands a developer would, and is held to the files its role may change.
- About 5,600 lines of parser and tests are gone, and with them the reviews that chased spellings.
- A determined agent can still type a command the permission rules do not match, or write outside the checkout. That is accepted: the change it makes to the
  repository is judged at its stop, and CI and the merge stand behind it.
