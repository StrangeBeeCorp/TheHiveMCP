# Run the unsigned binary on macOS

This page holds the steps that let a GUI host launch the unsigned macOS binary of TheHiveMCP past Gatekeeper.

## Clear the Gatekeeper quarantine

macOS binaries are currently unsigned. When a GUI host (VS Code, Claude Desktop) or Gatekeeper tries to launch the binary, you get:

> "Apple could not verify 'thehivemcp' is free of malware…"

Running it directly from the terminal works because the shell bypasses this check—but MCP hosts don't. Remove the quarantine attribute once, after extracting
the binary (the archive names it `thehivemcp-darwin-<arch>`; the guides rename it `thehivemcp`):

```bash
xattr -d com.apple.quarantine /path/to/thehivemcp
chmod +x /path/to/thehivemcp
```

(Alternatively: **System Settings → Privacy & Security → "Open Anyway"** after the first blocked launch.) The binary will then launch from any host.
