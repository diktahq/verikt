<!-- edikt:start — managed by edikt, do not edit manually -->
## Edikt

### Project
"Your architecture, in every agent session." — verikt is Agentic Engineering Infrastructure: a Go CLI for scaffolding, analyzing, guiding AI agents, and enforcing code architecture.

### Before Writing Code
1. Read `.claude/soul.md` for project context
2. Rules are enforced automatically via `.claude/rules/`
3. If a plan is active, read it in `docs/product/plans/` — check progress table for current state

### Build & Test Commands
```
# Build
go build ./...

# Test
go test ./...

# Lint
golangci-lint run

# Vet
go vet ./...
```

### Edikt Commands
When the user asks any of the following, run the corresponding command automatically:

| If the user asks... | Run |
|---------------------|-----|
| "what's our status?", "where are we?", "project status" | `/edikt:status` |
| "what's next?", "what should we do next?", "next steps" | `/edikt:status` |
| "load context", "remind yourself", "what's this project?" | `/edikt:context` |
| "create a plan", "let's plan this", "plan for X" | `/edikt:plan` |
| "save this decision", "record this", "capture that" | `/edikt:adr` |
| "add an invariant", "that's a hard rule", "never do X" | `/edikt:invariant` |
| "write a PRD", "document this feature", "requirements for X" | `/edikt:prd` |

### After Compaction
If context was compacted, re-read the active plan file in `docs/product/plans/`. The progress table is the persistent state — it tells you what's done and what's next.
<!-- edikt:end -->
