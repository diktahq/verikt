# Design Session: Config Presets & Inheritance

_Status: Design exploration — not implemented_
_Date: 2026-03-11_

How `verikt.yaml` presets and config inheritance would work. ESLint-style `extends` for sharing architecture rules across teams and orgs. Targets ICP-2 (engineering leaders wanting consistency at scale).

---

## 1. How presets are referenced

In `verikt.yaml`:

```yaml
# Single preset
extends:
  - verikt/api-strict

# Multiple presets (merged in order, later overrides earlier)
extends:
  - verikt/minimal
  - github.com/myorg/verikt-presets/payments-team

# Local preset file
extends:
  - ./presets/our-standards.yaml
```

Resolution order: presets are loaded left-to-right, then the project's own `verikt.yaml` fields override everything. Like ESLint — the most specific config wins.

---

## 2. What a preset file looks like

A preset is a YAML file with the same structure as `verikt.yaml`, minus `language` and `architecture` (those are project-specific, not shareable).

### `verikt/strict` — production API hardening

```yaml
# preset: verikt/strict
# Strict rules for production APIs. Tight function limits, required
# directories, naming conventions, and all critical capability warnings.

rules:
  functions:
    max_lines: 50
    max_params: 3
    max_return_values: 2
  structure:
    required_dirs:
      - domain/
      - port/
      - service/
      - adapter/
    forbidden_dirs:
      - utils/
      - helpers/
      - common/
      - shared/
  naming:
    - pattern: "adapter/*repo/*"
      must_end_with: "Repo"
    - pattern: "adapter/*handler/*"
      must_end_with: "Handler"
    - pattern: "port/inbound/*"
      must_end_with: "UseCase"

check:
  exclude:
    - "generated/**"
    - "vendor/**"
    - "*.pb.go"

guide:
  mode: audit
```

### `verikt/minimal` — lightweight defaults

```yaml
# preset: verikt/minimal
# Relaxed rules for prototypes, internal tools, or early-stage projects.
# Catches the worst violations without getting in the way.

rules:
  functions:
    max_lines: 100
    max_params: 5
    max_return_values: 3
  structure:
    required_dirs: []
    forbidden_dirs:
      - utils/
      - helpers/

guide:
  mode: passive
```

### `verikt/api-production` — full production stack

```yaml
# preset: verikt/api-production
# Recommended capabilities and rules for a production HTTP API.
# Extends strict and adds capability suggestions.

extends:
  - verikt/strict

capabilities:
  - platform
  - bootstrap
  - http-api
  - health
  - request-id
  - graceful
  - docker
  - testing
  - linting
  - makefile

rules:
  functions:
    max_lines: 60
    max_params: 4
```

---

## 3. Team/org preset (git repo)

A team creates a repo at `github.com/myorg/verikt-presets` with:

```
verikt-presets/
├── payments-team.yaml
├── platform-team.yaml
└── shared-base.yaml
```

### `payments-team.yaml`

```yaml
# Team-specific rules for the payments domain.
# Every payments service must have audit logging, idempotency,
# and encryption. No exceptions.

extends:
  - ./shared-base.yaml

capabilities:
  - audit-log
  - idempotency
  - encryption
  - auth-jwt

rules:
  functions:
    max_lines: 40
    max_params: 3
  structure:
    required_dirs:
      - domain/
      - port/
      - service/
      - adapter/
    forbidden_dirs:
      - utils/
      - helpers/

check:
  exclude:
    - "generated/**"
    - "proto/**"
```

### `shared-base.yaml`

```yaml
# Org-wide baseline. Every service extends this.

rules:
  functions:
    max_lines: 80
    max_params: 4
    max_return_values: 2
  structure:
    forbidden_dirs:
      - utils/
      - helpers/
      - common/

guide:
  mode: passive
```

Referenced in a project:

```yaml
language: go
architecture: hexagonal
extends:
  - github.com/myorg/verikt-presets/payments-team
capabilities:
  - platform
  - bootstrap
  - http-api
  - postgres
  - migrations
```

The project gets: shared-base rules → payments-team rules → project overrides. Capabilities merge (union). Rules override (project wins).

---

## 4. Local preset (monorepo)

For monorepos where all services share a base config:

```
monorepo/
├── presets/
│   └── service-base.yaml
├── services/
│   ├── orders/
│   │   └── verikt.yaml      # extends: [../../presets/service-base.yaml]
│   ├── payments/
│   │   └── verikt.yaml      # extends: [../../presets/service-base.yaml]
│   └── notifications/
│       └── verikt.yaml      # extends: [../../presets/service-base.yaml]
```

### `presets/service-base.yaml`

```yaml
rules:
  functions:
    max_lines: 60
    max_params: 4
  structure:
    forbidden_dirs:
      - utils/
      - helpers/

guide:
  mode: passive

check:
  exclude:
    - "generated/**"
```

### `services/orders/verikt.yaml`

```yaml
language: go
architecture: hexagonal
extends:
  - ../../presets/service-base.yaml
capabilities:
  - platform
  - bootstrap
  - http-api
  - postgres
  - migrations
  - uuid
```

---

## 5. Merge semantics

| Field | Merge strategy |
|-------|---------------|
| `language` | Not inherited — must be set in project |
| `architecture` | Not inherited — must be set in project |
| `capabilities` | Union — preset capabilities + project capabilities |
| `components` | Override — project replaces preset entirely if present |
| `rules.functions` | Override per field — project values override preset values |
| `rules.structure.required_dirs` | Union — both lists merged |
| `rules.structure.forbidden_dirs` | Union — both lists merged |
| `rules.naming` | Append — preset rules + project rules |
| `check.exclude` | Union — both lists merged |
| `guide.mode` | Override — project wins |
| `extends` | Chain — presets can extend other presets (max depth: 10) |

---

## 6. Resolution protocol

1. Parse project `verikt.yaml`
2. For each entry in `extends` (left to right):
   - If starts with `verikt/` → load from bundled presets
   - If starts with `./` or `../` → load from local file path (relative to verikt.yaml)
   - If starts with `github.com/` or contains `/` → fetch from git repo (clone to cache, read YAML)
3. Merge presets in order (left to right), then merge project config on top
4. Validate the final merged config

Git repos are cached in `~/.cache/verikt/presets/<hash>/` with a TTL (e.g., 1 hour). `verikt preset update` refreshes the cache.

---

## 7. CLI commands

```bash
# List available bundled presets
verikt preset list

# Show what a preset contains
verikt preset show verikt/strict

# Fetch and cache a git preset
verikt preset add github.com/myorg/verikt-presets/payments-team

# Refresh cached presets
verikt preset update
```
