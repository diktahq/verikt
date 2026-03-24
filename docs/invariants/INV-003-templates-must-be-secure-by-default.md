# INV-003: Templates Must Be Secure by Default

**Status:** active
**Date:** 2026-03-24

## Rule

Every template that verikt ships MUST produce code that is secure with zero configuration. A developer who scaffolds a project and deploys it without changing any defaults MUST NOT have an exploitable vulnerability.

Specifically:

1. **No hardcoded secrets or fallback credentials.** Templates MUST read secrets from environment variables and fail at startup if they are missing. NEVER provide a fallback value like `'change-me-in-production'` — it will ship to production.

2. **No permissive defaults that weaken security.** CORS MUST NOT default to `*`. gRPC reflection MUST default to `false`. TLS MUST NOT default to disabled. Auth middleware MUST validate tokens, not just check they exist.

3. **No unpinned dependencies.** Docker images MUST pin to a major version, not `:latest`. Go modules and npm packages MUST use fixed versions (caret ranges acceptable for npm given lockfiles).

4. **Auth templates MUST complete the verification.** A middleware that extracts a token MUST validate it (signature, expiry, issuer). A callback handler MUST verify the CSRF state parameter. A token exchange MUST use URL-encoded parameters, not string interpolation.

5. **Generated code MUST use parameterized queries.** No template may produce SQL via string concatenation or interpolation.

## Enforcement

- `TestTemplates_NoInsecurePatterns` in `internal/security/` scans all templates against a deny-list of insecure patterns. This runs on every CI build.
- The deny-list grows as new anti-patterns are discovered. Adding a pattern to the deny-list is a one-line change; fixing the template is the real work.
- Security review is required for any new auth, database, or network capability template before merge.

## Rationale

Templates are force multipliers. A single insecure pattern in a template ships to every project that uses it. Unlike application bugs that affect one codebase, template security bugs affect every scaffolded project — past and future. The blast radius is the entire user base.

EXP-12d showed that agents follow the patterns they're given. If verikt ships insecure patterns in templates, agents will reproduce them. The templates ARE the security posture of every generated project.

## Consequences of Violation

- Every project scaffolded with the insecure template inherits the vulnerability
- Users trust verikt to produce production-grade code — a single CVE-worthy default destroys that trust
- The vulnerability compounds: each `verikt new` creates another affected project
