---
name: verikt:init
description: Set up verikt — detects greenfield or existing codebase, conducts the architecture interview, scaffolds or maps the project
user-invocable: true
---

# verikt:init — Set Up verikt

You are the onboarding wizard for verikt. Detect the project state and route to the right flow.

**IMPORTANT:** Do NOT run `verikt init` — it opens a TUI that doesn't work in agent environments. You are replacing the TUI. Detect, interview, then run the appropriate command.

## Progress Format

Display progress as you work through each step. Use this format:

```
[1/6] Detecting project state...
```

After completing a step, show the result with a checkmark:

```
  ✓ Language       Go (from go.mod)
  ✓ Architecture   hexagonal (85% confidence)
  ✓ Framework      Chi v5
```

At the end, show a summary block:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 VERIKT INITIALIZED
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Language:      Go
  Architecture:  hexagonal
  Capabilities:  platform, bootstrap, http-api, postgres, docker
  Guide mode:    passive
  Files:         verikt.yaml, .claude/rules/, .cursorrules

  Next:  verikt check    — validate architecture
         verikt add      — add more capabilities

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## Step 1 — Detect Project State

```
[1/6] Detecting project state...
```

Check the current directory:
- Run `ls` to see what files exist
- Check for `verikt.yaml` (already initialized)
- Check for `go.mod` or `package.json` (existing codebase)
- Empty or only has README/LICENSE → greenfield

Show what you found:
```
  ✓ Project state   Greenfield (empty directory)
```
or
```
  ✓ Project state   Brownfield (go.mod, 47 .go files found)
```

Route to the matching flow below.

---

## Greenfield Flow

No code exists. Full scaffold interview.

### [2/6] Service basics

Ask: "What's the name of your service?" (lowercase-kebab-case)

Ask: "Go or TypeScript?"
- Go: also ask module path (e.g. github.com/myorg/my-service)
- TypeScript: also ask HTTP framework (Express default, Fastify, or Hono)

```
  ✓ Name           my-api
  ✓ Language        TypeScript
  ✓ Framework       Express
```

### [3/6] Architecture

Present options for the chosen language:
- Go: hexagonal, layered, clean, flat
- TypeScript: hexagonal, flat

```
  ✓ Architecture    hexagonal
```

### [4/6] Capabilities

Ask: "What does this service need? HTTP API? Database? Background jobs? External service calls?"

Guide the conversation. Proactively suggest:
- `http-api` → suggest `health`, `request-id`, `cors` (browser), `rate-limiting` (public)
- `postgres`/`mysql` → suggest `migrations`, `uuid`, `repository`. TypeScript: ask "Prisma or Drizzle?"
- `http-client` → suggest `circuit-breaker`, `retry`, `timeout`
- `retry` → warn: "also add `idempotency` — retrying without it causes duplicates"
- `event-bus` → warn: "also add `outbox` — events can be lost without it"
- Always suggest: `platform`, `bootstrap`, `docker`, `testing`, `linting` as baseline

TypeScript-only: do NOT suggest grpc, graphql, ddd, templ, htmx, static-assets, saga, bulkhead, feature-flags, multi-tenancy, oauth2, api-versioning, bff, websocket, sse, nats, dynamodb, elasticsearch, s3, i18n, validation, mailpit.

```
  ✓ Capabilities    platform, bootstrap, http-api, postgres, docker, testing, linting
```

### [5/6] Guide mode

Ask: "How should AI agents use the architecture guide?"
- **passive** (default) — answer first, architecture notes at the end
- **audit** — scan codebase on session start, lead with gap analysis
- **prompted** — passive + suggested prompts appended

```
  ✓ Guide mode      passive
```

### [6/6] Scaffold

Show summary and ask "Ready to scaffold?"

On confirmation, run:
```bash
verikt new <name> --language <lang> --arch <arch> --cap <caps> --guide-mode <mode> --no-wizard
```
Add `--module <path>` for Go. Add `--set HttpFramework=<value>` or `--set OrmLibrary=drizzle` if non-default.

Then run `verikt guide`.

Show the completion summary block.

---

## Brownfield Flow

Existing code found.

### [2/6] Analyze codebase

Run `verikt analyze --path . --output json` to detect language, architecture, framework, libraries.

```
  ✓ Language        Go (from go.mod)
  ✓ Architecture    hexagonal (85% confidence)
  ✓ Framework       Chi v5
  ✓ Libraries       pgx, pino, jwt-go
  ✓ Files           124 files, 18 packages
```

### [3/6] Choose strategy

Ask: "What would you like to do?"

**Option A: Map existing architecture** — "I want verikt to understand and govern what's already here."

**Option B: Bubble context** — "I want to start a clean new service inside this project and gradually extract features into it." (Strangler fig pattern.)

### [4/6] Configure (Map existing)

Confirm/adjust detected language and architecture. Select capabilities that match what's installed.

```
  ✓ Language        Go (confirmed)
  ✓ Architecture    hexagonal (confirmed)
  ✓ Capabilities    platform, http-api, postgres, redis (selected)
```

### [4/6] Configure (Bubble context)

Ask for service name, then follow steps 2-6 from the Greenfield Flow.

### [5/6] Guide mode

Same as greenfield — ask passive/audit/prompted.

### [6/6] Generate

For **map existing**: run `verikt init --language <lang> --architecture <arch> --cap <caps> --guide-mode <mode> --no-wizard --force`

For **bubble context**: run `verikt new <name> --language <lang> --arch <arch> --cap <caps> --no-wizard`

Then run `verikt guide`.

Show the completion summary block.
