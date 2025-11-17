# Documentation

This directory contains comprehensive documentation for TheHiveMCP.

## Contents

- **[CHANGELOG.md](CHANGELOG.md)** - Project changelog following semantic versioning
- **[images/](images/)** - Documentation images and assets
- **[tools/](tools/)** - Detailed tool documentation for each MCP tool

## Release Documentation

For release management:
- Version information is managed through Git tags and the `version/` package
- Changelog is maintained in this folder (not at project root)
- Release notes are auto-generated from changelog during the release process

## Contributing

When adding new features or making changes:
1. Update the relevant tool documentation in `tools/`
2. Add changelog entries to `CHANGELOG.md` under "Unreleased"
3. Update version information will be handled automatically by the release process
