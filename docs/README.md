# Documentation

This directory contains comprehensive documentation for TheHiveMCP.

## Contents

- **[how-to/](how-to/)** - Goal-oriented guides for specific setup tasks
- **[tools/](tools/)** - Detailed tool documentation for each MCP tool
- **[images/](images/)** - Documentation images and assets

## How-to guides

- **[Set up TheHiveMCP with Claude Code](how-to/setup-claude-code.md)** — connect the server to the Anthropic CLI, with config precedence and troubleshooting.
- **[Run on Windows from source](how-to/run-on-windows-from-source.md)** — compile the server locally to sidestep the SmartScreen "unknown publisher" prompt. A
  fallback for when the signed release binary is not an option.

## Release Documentation

For release management:

- Version information is managed through Git tags and the `version/` package
- Release notes are auto-generated during the release process

## Contributing

When adding new features or making changes:

1. Update the relevant tool documentation in `tools/`
2. Version information will be handled automatically by the release process
