# verikt

**Your architecture, in every agent session.**

AI agents don't know your architecture. They write correct syntax but put code in the wrong layers, skip production patterns, and drift from your conventions. verikt closes that gap — declare your architecture and capabilities once, and every agent session starts with the right context.

**[Documentation](https://verikt.dev/)** · **[Quick Start with AI Agents](https://verikt.dev/getting-started/ai-agents/)** · **[Quick Start with CLI](https://verikt.dev/getting-started/cli/)** · **[Capabilities Matrix](https://verikt.dev/reference/capabilities-matrix/)**

## Install

```bash
# Homebrew (recommended)
brew install diktahq/tap/verikt
```

### Pinned and verified, by hand

This is the only route where everything you run is fixed to a release: you download the archive for one version and check it against that release's `checksums.txt` yourself. Archives are named `verikt_<version>_<os>_<arch>.tar.gz`, for `darwin` or `linux` on `amd64` or `arm64`.

```bash
VERSION=v0.2.0
ARCHIVE="verikt_${VERSION#v}_darwin_arm64.tar.gz"
curl -fsSLO "https://github.com/diktahq/verikt/releases/download/${VERSION}/${ARCHIVE}"
curl -fsSLO "https://github.com/diktahq/verikt/releases/download/${VERSION}/checksums.txt"
grep "  ${ARCHIVE}$" checksums.txt | shasum -a 256 -c -   # or: sha256sum -c -
tar xzf "${ARCHIVE}" verikt && sudo mv verikt /usr/local/bin/
```

### Install script (macOS and Linux)

```bash
curl -fsSL https://verikt.dev/install.sh | bash
```

The script resolves the latest release to a concrete tag, downloads that release's archive from GitHub, and checks it against the release's `checksums.txt`. It refuses to install if the checksum entry is missing, the hash doesn't match, or neither `sha256sum` nor `shasum` is available. It installs to `/usr/local/bin` by default; set `VERIKT_INSTALL_DIR` to an absolute path to change that.

`VERIKT_VERSION` pins the release the script installs:

```bash
curl -fsSL https://verikt.dev/install.sh | VERIKT_VERSION=v0.2.0 bash
```

That pins the archive, not the script. `install.sh` itself is fetched live from verikt.dev each time, so piping it into `bash` runs whatever is published there at that moment. To review it first, download it, read it, then run it:

```bash
curl -fsSLo install.sh https://verikt.dev/install.sh
less install.sh
VERIKT_VERSION=v0.2.0 bash install.sh
```

The script doesn't touch your agent configuration unless you ask. Pass `--setup` to run `verikt setup` right after installing, or run it yourself later:

```bash
curl -fsSL https://verikt.dev/install.sh | bash -s -- --setup
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
| **Enforce** | `verikt check` | Validates code against architecture rules — 14 anti-pattern detectors, `--diff main` for CI |

## Language Support

**Go** — 4 architecture patterns, 65 capabilities across 10 categories. Fully supported.

**TypeScript/Node.js** — 2 architecture patterns (hexagonal, flat), 39 capabilities. HTTP framework choice: Express, Fastify, or Hono. Scaffolding and agent context are on par with Go; `check` covers dependency rules and structure only.

Depth differs per language — see [docs/reference/language-support.md](docs/reference/language-support.md).

## License

[Elastic License 2.0 (ELv2)](LICENSE)
