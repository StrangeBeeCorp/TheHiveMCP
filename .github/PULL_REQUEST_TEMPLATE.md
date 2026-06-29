<!-- Thanks for contributing to TheHiveMCP! -->

## Summary

<!-- What does this change and why? -->

## Release impact

<!-- This feeds the re-test policy at release time — see RELEASING.md. -->

- [ ] This change can affect model-facing behavior (tools, tool descriptions,
      auth/authz, prompts). If checked, the next release **must** re-run:
  - [ ] Accuracy suite
  - [ ] Security suite
- [ ] No model-facing behavior change (refactor / dependencies / docs / CI) —
      no re-run required.

## Checklist

- [ ] Tests pass (`make test`)
- [ ] Docs updated if behavior changed
- [ ] User-facing / breaking / security impact noted for the release notes
