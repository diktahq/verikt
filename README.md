# archway

**Architecture-aware service composer and enforcer.**

Compose production-grade services from architecture patterns and capabilities. Feed AI agents your architecture context so they write structurally correct code from the first prompt.

**[Documentation](https://archway.dcsg.me/)** · **[Quick Start with AI Agents](https://archway.dcsg.me/getting-started/ai-agents/)** · **[Quick Start with CLI](https://archway.dcsg.me/getting-started/cli/)** · **[Capabilities Matrix](https://archway.dcsg.me/reference/capabilities-matrix/)**

## Install

```bash
# Homebrew
brew install dcsg/tap/archway

# Go install
go install github.com/dcsg/archway/cmd/archway@latest
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
| **Compose** | `archway new` | Scaffolds production-ready services from architecture + capabilities |
| **Analyze** | `archway analyze` | Detects architecture patterns in existing codebases |
| **Enforce** | `archway check` | Validates code against architecture rules — 11 anti-pattern detectors |

## What's Inside

- **4 architectures:** hexagonal, flat, layered, clean
- **63 capabilities** across 10 categories (transport, data, resilience, security, patterns, observability, infrastructure, quality, and more)
- **Smart wizard** that suggests missing capabilities and warns about risky combinations
- **AI-native** — `archway guide` outputs markdown that AI agents read before writing code

## Language Support

Go is fully supported. TypeScript/Node is next. Archway's provider model makes it straightforward to add any language.

## License

[Elastic License 2.0 (ELv2)](LICENSE)
