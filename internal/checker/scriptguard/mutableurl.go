package scriptguard

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// RuleMutableInstallURL flags installs and dependencies fetched from a
// location whose content can change under the same reference.
const RuleMutableInstallURL = "mutable_install_url"

var (
	// pipeToShell is curl/wget piped into a shell, allowing sudo/env
	// wrappers and VAR=value prefixes before the shell. "| shasum" and
	// "| sha256sum" do not match.
	pipeToShell = regexp.MustCompile(`\b(?:curl|wget)\b[^|;&\n]*\|\s*(?:(?:sudo|(?:/(?:usr/)?bin/)?env)(?:\s+-\S+)*\s+)*(?:[A-Za-z_][A-Za-z0-9_]*=(?:"[^"]*"|'[^']*'|\S*)\s+)*(?:/(?:usr/)?(?:local/)?bin/)?(?:ba|z|da|k|a)?sh(?:[\s;&|)]|$)`)

	// shellOfDownload is `bash <(curl ...)`, `source <(curl ...)`,
	// `sh -c "$(curl ...)"` (any path, any -…c… flag cluster, $() or
	// backticks) and `eval "$(curl ...)"`.
	shellOfDownload = regexp.MustCompile(`(?:^|[\s;&|(])(?:` +
		`(?:(?:/(?:usr/)?(?:local/)?bin/)?(?:ba|z|da|k|a)?sh|source|\.)\s[^;&\n]*<\(\s*(?:curl|wget)\b|` +
		`(?:(?:/(?:usr/)?(?:local/)?bin/)?(?:ba|z|da|k|a)?sh\s+(?:[-+]\S+\s+)*-[A-Za-z]*c[A-Za-z]*|eval)\s+["']?(?:\$\(|` + "`" + `)\s*(?:curl|wget)\b)`)

	// latestReleaseDownload is GitHub's floating "latest release" asset URL.
	latestReleaseDownload = regexp.MustCompile(`/releases/latest/download/`)

	// rawBranchURL is a raw.githubusercontent.com URL on a moving branch.
	rawBranchURL = regexp.MustCompile(`raw\.githubusercontent\.com/[^/\s"']+/[^/\s"']+/(?:refs/heads/)?(?:main|master|HEAD)/([^\s"')]*)`)

	// usesBranch is a workflow `uses:` pinned to a branch.
	usesBranch = regexp.MustCompile(`^\s*(?:-\s*)?uses\s*:\s*['"]?[^@\s'"]+@(?:main|master|HEAD)['"]?\s*$`)

	// dockerFrom is a Dockerfile FROM line: flags, image, optional stage name.
	dockerFrom = regexp.MustCompile(`(?i)^\s*FROM\s+((?:--\S+\s+)*)([^\s#]+)(?:\s+AS\s+([A-Za-z0-9._-]+))?`)
)

// staticAssetExts are skipped for raw.githubusercontent.com: README badges
// and images on main are not installs.
var staticAssetExts = []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico"}

// scanMutableInstallURL applies mutable_install_url to f.
func scanMutableInstallURL(f *scriptFile) {
	switch f.kind {
	case KindWorkflow, KindLefthook, KindShell, KindMakefile, KindDockerfile, KindCIConfig, KindMarkdown:
	default:
		return
	}
	stages := map[string]bool{}
	for _, ll := range f.logical {
		reasons := mutableReasons(f.kind, ll, stages)
		if len(reasons) == 0 {
			continue
		}
		msg := strings.Join(reasons, "; ") + ". Pin a version tag, commit SHA or image digest, and verify downloads with a checksum"
		f.report(RuleMutableInstallURL, msg, ll)
	}
}

// mutableReasons lists every mutable reference on one logical line. stages
// accumulates Dockerfile stage names seen so far.
func mutableReasons(kind Kind, ll logicalLine, stages map[string]bool) []string {
	var reasons []string
	text := ll.text
	if unquotedPipeToShell(ll.cmd) || pipelineIntoShell(ll.cmd) || shellOfDownload.MatchString(ll.cmd) {
		reasons = append(reasons, "downloads a script and runs it without pinning or verifying it")
	}
	if latestReleaseDownload.MatchString(text) {
		reasons = append(reasons, "downloads from /releases/latest/, which changes with every release")
	}
	if kind != KindMarkdown && rawBranchHit(text) {
		reasons = append(reasons, "fetches raw.githubusercontent.com from a branch (main/master/HEAD), which changes with every push")
	}
	if kind == KindWorkflow && usesBranch.MatchString(text) {
		reasons = append(reasons, "uses an action or reusable workflow pinned to a branch")
	}
	if kind == KindDockerfile {
		if img, ok := mutableBaseImage(text, stages); ok {
			reasons = append(reasons, fmt.Sprintf("FROM %s has no fixed tag (untagged or :latest)", img))
		}
	}
	return reasons
}

// unquotedPipeToShell reports a download piped into a shell where the pipe
// itself is outside quotes: `echo "curl ... | bash"` prints an install hint,
// it does not run one.
func unquotedPipeToShell(text string) bool {
	matches := pipeToShell.FindAllStringIndex(text, -1)
	if matches == nil {
		return false
	}
	sc := scanShell(text)
	for _, m := range matches {
		if p := strings.IndexByte(text[m[0]:m[1]], '|'); p >= 0 && !sc.quoted[m[0]+p] {
			return true
		}
	}
	return false
}

// pipelineIntoShell reports a download that reaches a shell reading stdin
// through intermediate stages: `curl ... | tee log | bash`. Stages are split
// outside quotes, so an echoed install hint is not a pipeline.
func pipelineIntoShell(text string) bool {
	for _, cmd := range splitTopLevel(text, listSep) {
		downloaded := false
		for _, stage := range splitTopLevel(cmd, pipeSep) {
			fields := stripCommandPrefixes(strings.Fields(stage))
			if len(fields) == 0 {
				continue
			}
			switch base := path.Base(fields[0]); {
			case base == "curl" || base == "wget":
				downloaded = true
			case downloaded && shellNames[base] && shellReadsStdin(fields[1:]):
				return true
			}
		}
	}
	return false
}

// shellNames are the shells a download can be piped into.
var shellNames = setOf("sh", "bash", "zsh", "dash", "ksh", "ash")

// shellReadsStdin reports whether a shell with these arguments runs its
// standard input: no script file and no -c command string, or -s.
func shellReadsStdin(args []string) bool {
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--":
			return len(args) == 1 // a script file may follow "--"
		case a == "-":
			return true
		case isShortOption(a, '-') && strings.Contains(a, "s"):
			return true
		case isShortOption(a, '-') && strings.Contains(a, "c"):
			return false
		}
		n, ok := shellOptionLen(args)
		if !ok {
			return false // a script file: stdin is its input, not its code
		}
		args = args[n:]
	}
	return true
}

func rawBranchHit(text string) bool {
	for _, m := range rawBranchURL.FindAllStringSubmatch(text, -1) {
		if !hasStaticAssetExt(m[1]) {
			return true
		}
	}
	return false
}

func hasStaticAssetExt(p string) bool {
	p = strings.ToLower(p)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	for _, ext := range staticAssetExts {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

// mutableBaseImage parses a FROM line and reports an image that is untagged
// or tagged :latest without a digest. scratch, earlier stage names and
// $ARG images (unless :latest) are exempt. The tag is looked for after the
// last "/", so a registry port (localhost:5000/app) is not a tag.
func mutableBaseImage(text string, stages map[string]bool) (string, bool) {
	m := dockerFrom.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	img := m[2]
	defer func() {
		if m[3] != "" {
			stages[strings.ToLower(m[3])] = true
		}
	}()
	lower := strings.ToLower(img)
	if lower == "scratch" || stages[lower] || strings.Contains(img, "@sha256:") {
		return "", false
	}
	base := lower[strings.LastIndex(lower, "/")+1:]
	if strings.HasSuffix(base, ":latest") {
		return img, true
	}
	if strings.HasPrefix(img, "$") || strings.Contains(base, ":") {
		return "", false
	}
	return img, true
}
