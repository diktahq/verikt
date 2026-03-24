# Changelog

All notable changes to verikt are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/).

## [0.1.0] — 2026-03-23

First public release of verikt. Previously developed as "archway" — renamed to verikt as part of the dikta platform (diktahq).

### Added
- **Go language provider** — 63 capabilities across 10 categories (transport, data, resilience, patterns, security, observability, infrastructure, quality, platform, frontend). 4 architecture patterns: hexagonal, layered, clean, flat.
- **TypeScript/Node.js provider** — 39 capabilities across data, resilience, security, patterns, and infrastructure. Two architectures: hexagonal and flat. HTTP framework choice: Express, Fastify, or Hono.
- **Rust analysis engine** — tree-sitter-based import graph analysis for Go and TypeScript. Embedded via `//go:embed`, extracted to user cache on first run. Protobuf communication over stdin/stdout. Cross-compiled for darwin-arm64, darwin-amd64, linux-arm64, linux-amd64.
- `verikt new` with interactive wizard and `--no-wizard` mode.
- `verikt add` for adding capabilities to existing projects.
- `verikt check` with 11 AST-based detectors (dependency violations, anti-patterns, function metrics, structure checks). Powered by Rust engine import graph.
- `verikt guide` generating architecture context for Claude Code, Cursor, Copilot, and Windsurf. Includes governance checkpoint validated by EXP-10.
- `verikt init` — single onboarding entry point. Detects greenfield (empty directory → scaffold wizard) vs brownfield (existing code → analyze + map or bubble context).
- `verikt analyze` for detecting architecture patterns in existing codebases.
- `verikt setup` for registering with AI agents. Installs skills globally, locally, or both.
- `/verikt:init` skill for Claude Code with `[n/6]` progress indicators.
- Smart suggestions, capability warnings, feature-flag template engine, proxy rules, decision gates.
- **Drizzle ORM** as alternative to Prisma for TypeScript postgres and mysql capabilities.
- **Squirrel** query builder and **sqlc** capabilities for Go.
- Node.js version-gated features via `features.yaml`. Default Node version: 22 (Active LTS).
- Website with Starlight docs, 10 experiments, glossary, capability pages, architecture comparisons.
- `brew install diktahq/tap/verikt` distribution.
