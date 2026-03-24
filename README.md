# verikt

**Your architecture, in every agent session.**

AI agents don't know your architecture. They write correct syntax but put code in the wrong layers, skip production patterns, and drift from your conventions. verikt closes that gap — declare your architecture and capabilities once, and every agent session starts with the right context.

**[Documentation](https://verikt.dev/)** · **[Quick Start with AI Agents](https://verikt.dev/getting-started/ai-agents/)** · **[Quick Start with CLI](https://verikt.dev/getting-started/cli/)** · **[Capabilities Matrix](https://verikt.dev/reference/capabilities-matrix/)**

## Install

```bash
# Homebrew (recommended)
brew install diktahq/tap/verikt

# Install script (macOS/Linux)
curl -sSL https://verikt.dev/install.sh | bash
```

## Quick Start

```bash
# Interactive wizard — walks you through everything
verikt new my-service

# Non-interactive
verikt new my-api --arch hexagonal \
  --cap platform,bootstrap,http-api,postgres,uuid,health,docker \
  --module github.com/myorg/my-api \
  --no-wizard
```

## The Four Pillars

| Pillar | Command | What It Does |
|--------|---------|-------------|
| **Guide** | `verikt guide` | Generates architecture context for AI agents (Claude Code, Cursor, Copilot, Windsurf) |
| **Compose** | `verikt new` | Scaffolds services from architecture + capabilities — health checks, graceful shutdown, observability wired by default |
| **Analyze** | `verikt analyze` | Detects architecture patterns in existing codebases |
| **Enforce** | `verikt check` | Validates code against architecture rules — 11 anti-pattern detectors, `--diff main` for CI |

## Language Support

**Go** — 4 architecture patterns, 63 capabilities across 10 categories. Fully supported.

**TypeScript/Node.js** — 2 architecture patterns (hexagonal, flat), 39 capabilities. HTTP framework choice: Express, Fastify, or Hono.

## License

[Elastic License 2.0 (ELv2)](LICENSE)
