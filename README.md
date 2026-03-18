# archway

**Architecture-aware service composer and enforcer.**

AI agents don't know your architecture. They write correct syntax but put code in the wrong layers, skip production patterns, and drift from your conventions. Archway closes that gap — declare your architecture and capabilities once, and every agent session starts with the right context.

**[Documentation](https://archway.dcsg.me/)** · **[Quick Start with AI Agents](https://archway.dcsg.me/getting-started/ai-agents/)** · **[Quick Start with CLI](https://archway.dcsg.me/getting-started/cli/)** · **[Capabilities Matrix](https://archway.dcsg.me/reference/capabilities-matrix/)**

## Install

```bash
# Homebrew (recommended)
brew install dcsg/tap/archway

# Install script (macOS/Linux)
curl -sSL https://archway.dcsg.me/install.sh | bash
```

## Quick Start

```bash
# Interactive wizard — walks you through everything
archway new my-service

# Non-interactive
archway new my-api --arch hexagonal \
  --cap platform,bootstrap,http-api,postgres,uuid,health,docker \
  --module github.com/myorg/my-api \
  --no-wizard
```

## The Four Pillars

| Pillar | Command | What It Does |
|--------|---------|-------------|
| **Guide** | `archway guide` | Generates architecture context for AI agents (Claude Code, Cursor, Copilot, Windsurf) |
| **Compose** | `archway new` | Scaffolds services from architecture + capabilities — health checks, graceful shutdown, observability wired by default |
| **Analyze** | `archway analyze` | Detects architecture patterns in existing codebases |
| **Enforce** | `archway check` | Validates code against architecture rules — 11 anti-pattern detectors, `--diff main` for CI |

## Language Support

Go is fully supported with 4 architecture patterns and 63 capabilities across 10 categories. TypeScript/Node is next — a new language provider requires tree-sitter grammar bindings and query definitions.

## License

[Elastic License 2.0 (ELv2)](LICENSE)
