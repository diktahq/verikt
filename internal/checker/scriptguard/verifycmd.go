package scriptguard

import (
	"path"
	"regexp"
	"strings"
)

// verifyTools are commands that exist to fail when something is wrong:
// test runners, linters, type checkers, scanners and migration tools.
var verifyTools = setOf(
	"pytest", "unittest", "tox", "nox", "jest", "vitest", "mocha", "playwright", "cypress", "rspec",
	"phpunit", "ctest", "bats", "golangci-lint", "staticcheck", "govulncheck", "revive", "eslint",
	"biome", "tsc", "mypy", "pyright", "ruff", "flake8", "pylint", "rubocop", "shellcheck", "hadolint",
	"yamllint", "actionlint", "markdownlint", "stylelint", "tflint", "checkov", "verikt", "semgrep",
	"gosec", "trivy", "bandit", "gitleaks", "migrate", "alembic", "goose", "atlas", "flyway", "liquibase",
)

// cleanupCommands are never verification commands, whatever their arguments
// mention: `rm -rf .pytest_cache || true` is ordinary best-effort cleanup.
var cleanupCommands = setOf(
	"rm", "rmdir", "mkdir", "kill", "pkill", "killall", "umount", "unlink", "tput", "pgrep",
	"docker", "podman",
)

// verifySegments and setupSegments classify script and target names split on
// - _ . : such as run-tests.sh, test-short, migrate-up or clean-test-db.sh.
var (
	verifySegments = setOf("test", "tests", "lint", "check", "checks", "verify", "vet", "typecheck",
		"migrate", "migration", "migrations", "e2e", "ci", "audit")
	setupSegments = setOf("clean", "cleanup", "teardown", "reset", "setup", "down")
)

// commandPrefixes are wrappers that run the next word as the command.
var commandPrefixes = setOf("sudo", "time", "command", "exec", "nice", "nohup", "env", "then", "do",
	"else", "!", "{", "(", "npx", "uvx", "bunx", "pnpx")

// twoWordPrefixes are wrappers spelled as two words.
var twoWordPrefixes = setOf("pnpm exec", "pnpm dlx", "yarn dlx", "yarn exec", "bundle exec", "poetry run",
	"uv run", "pipenv run", "python -m", "python3 -m")

var pkgScript = regexp.MustCompile(`^(?:test|lint|check|typecheck|verify|migrate)\b`)

// verificationCommand reports whether the command in fields is a test, lint,
// check or migration command, returning a short name for messages. extra
// holds check.silent_ci_swallow.extra_commands entries.
func verificationCommand(fields []string, extra []string) (string, bool) {
	fields = stripCommandPrefixes(fields)
	if len(fields) == 0 {
		return "", false
	}
	cmd, args := fields[0], fields[1:]
	base := path.Base(cmd)
	if (base == "bash" || base == "sh" || base == "zsh") && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
		base = path.Base(cmd)
	}
	switch {
	case cleanupCommands[base], base == "test", base == "[", base == "[[":
		return "", false
	case matchesExtra(cmd, args, extra):
		return base, true
	}
	if ok, known := subcommandVerifies(base, args); known {
		return strings.Join(fields[:min(len(fields), 2)], " "), ok
	}
	if verifyTools[base] {
		return base, true
	}
	// A path, a hyphenated name (migrate-up) or a script file is read by its
	// name. A bare snake_case word is usually a shell function the script
	// defines, such as an audit that runs cmd_check and goes on to report.
	if strings.Contains(cmd, "/") || strings.Contains(base, "-") || strings.HasSuffix(base, ".sh") {
		name := strings.TrimSuffix(strings.TrimSuffix(base, ".sh"), ".py")
		return base, isVerifyName(name)
	}
	return "", false
}

// subcommandVerifies handles tools whose subcommand decides whether they
// verify. known is false when base is not such a tool.
func subcommandVerifies(base string, args []string) (ok, known bool) {
	first := ""
	if len(args) > 0 {
		first = args[0]
	}
	switch base {
	case "go":
		return first == "test" || first == "vet", true
	case "cargo":
		return first == "test" || first == "clippy" || first == "check", true
	case "deno":
		return first == "test" || first == "lint" || first == "check", true
	case "npm", "pnpm", "yarn", "bun":
		if (first == "run" || first == "run-script") && len(args) > 1 {
			first = args[1]
		}
		return pkgScript.MatchString(first), true
	case "make", "gmake", "just", "task":
		return anyTarget(args), true
	case "mise":
		return (first == "run" || first == "r") && anyTarget(args[1:]), true
	case "mvn", "mvnw", "gradle", "gradlew", "dotnet":
		return anyArg(args, func(a string) bool { return a == "test" || a == "check" || a == "verify" }), true
	case "prisma", "drizzle-kit", "knex", "sqlx", "sequelize", "rails", "rake":
		return anyArg(args, func(a string) bool { return strings.HasPrefix(a, "migrate") || strings.HasPrefix(a, "db:migrate") }), true
	case "python", "python3":
		return strings.HasSuffix(first, "manage.py") && len(args) > 1 && (args[1] == "migrate" || args[1] == "test"), true
	case "terraform", "tofu":
		return first == "validate", true
	}
	return false, false
}

// stripCommandPrefixes drops env assignments, wrappers (sudo, npx, uv run,
// python -m ...) and Make recipe prefixes (@ - +) in front of the command.
func stripCommandPrefixes(fields []string) []string {
	if len(fields) > 0 {
		fields[0] = strings.TrimLeft(fields[0], "@-+")
		if fields[0] == "" {
			fields = fields[1:]
		}
	}
	for len(fields) > 0 {
		switch {
		case envAssign.MatchString(fields[0]):
			fields = fields[1:]
		case len(fields) > 1 && twoWordPrefixes[fields[0]+" "+fields[1]]:
			fields = fields[2:]
		case fields[0] == "timeout" && len(fields) > 1:
			fields = skipFlags(fields[1:])
			if len(fields) > 0 {
				fields = fields[1:] // the duration
			}
		case commandPrefixes[fields[0]]:
			fields = skipFlags(fields[1:])
		default:
			return fields
		}
	}
	return fields
}

func skipFlags(fields []string) []string {
	for len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
		fields = fields[1:]
	}
	return fields
}

// matchesExtra reports whether the command matches a configured extra
// command: same name (or path basename), and the entry's arguments as a
// prefix of args.
func matchesExtra(cmd string, args, extra []string) bool {
	for _, e := range extra {
		ef := strings.Fields(e)
		if len(ef) == 0 || (cmd != ef[0] && path.Base(cmd) != path.Base(ef[0])) || len(args) < len(ef)-1 {
			continue
		}
		match := true
		for i, a := range ef[1:] {
			if args[i] != a {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// anyTarget reports whether a make/just/task argument names a verification
// target. Flags and VAR=value arguments are ignored.
func anyTarget(args []string) bool {
	return anyArg(args, func(a string) bool {
		return !strings.HasPrefix(a, "-") && !strings.Contains(a, "=") && isVerifyName(a)
	})
}

// isVerifyName reports whether a script or target name has a verification
// segment (test, lint, check, migrate ...) and no setup/cleanup segment.
func isVerifyName(name string) bool {
	segs := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ':' || r == '/'
	})
	verify := false
	for _, s := range segs {
		if setupSegments[s] {
			return false
		}
		verify = verify || verifySegments[s]
	}
	return verify
}

func anyArg(args []string, pred func(string) bool) bool {
	for _, a := range args {
		if pred(a) {
			return true
		}
	}
	return false
}

func setOf(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}
