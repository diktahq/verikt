package guide

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrMalformedSection is returned when a shared file contains verikt's section
// markers in a shape verikt cannot safely rewrite: a start marker without an
// end marker (or vice versa), more than one section, or an end marker before
// the start marker. Callers must leave the file untouched when they see it.
var ErrMalformedSection = errors.New("malformed verikt section markers")

// SectionMarkers delimit the part of a shared file that verikt owns. Everything
// outside the markers belongs to the user and is never modified.
type SectionMarkers struct {
	Start string
	End   string
}

// GuideMarkers delimit the verikt section written by `verikt guide` into shared
// project files (AGENTS.md, .github/copilot-instructions.md, .windsurfrules) and
// the legacy .cursorrules verikt migrates away from.
var GuideMarkers = SectionMarkers{Start: sentinelStart, End: sentinelEnd}

// sectionSpan is the byte range [from, to) of a marked section, including both
// markers and one trailing newline after the end marker.
type sectionSpan struct {
	from, to int
}

// locate returns the span of the marked section, or nil when neither marker is
// present. Any other shape is ErrMalformedSection.
func (m SectionMarkers) locate(existing string) (*sectionSpan, error) {
	starts := strings.Count(existing, m.Start)
	ends := strings.Count(existing, m.End)
	if starts == 0 && ends == 0 {
		return nil, nil
	}
	if starts != 1 || ends != 1 {
		return nil, fmt.Errorf("%w: found %d %q and %d %q (expected exactly one of each)",
			ErrMalformedSection, starts, m.Start, ends, m.End)
	}
	from := strings.Index(existing, m.Start)
	endIdx := strings.Index(existing, m.End)
	if endIdx < from {
		return nil, fmt.Errorf("%w: %q appears before %q", ErrMalformedSection, m.End, m.Start)
	}
	to := endIdx + len(m.End)
	if to < len(existing) && existing[to] == '\n' {
		to++
	}
	return &sectionSpan{from: from, to: to}, nil
}

// ReplaceSection returns existing with verikt's section set to body. When the
// file has no section yet, the section is appended after the user's content.
// Content outside the markers is preserved byte for byte. Malformed markers
// return ErrMalformedSection so the caller never clobbers user content. A body
// that contains either marker returns ErrMarkerInBody: written, it would leave
// the file with marker counts every later run refuses as malformed.
func ReplaceSection(existing string, m SectionMarkers, body string) (string, error) {
	for _, marker := range []string{m.Start, m.End} {
		if strings.Contains(body, marker) {
			return "", fmt.Errorf("%w: %q (check verikt.yaml for a name or value containing it)", ErrMarkerInBody, marker)
		}
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	wrapped := m.Start + "\n" + body + m.End + "\n"

	span, err := m.locate(existing)
	if err != nil {
		return "", err
	}
	if span != nil {
		return existing[:span.from] + wrapped + existing[span.to:], nil
	}
	if existing == "" {
		return wrapped, nil
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + "\n" + wrapped, nil
}

// RemoveSection returns existing with verikt's section removed. removed reports
// whether a section was present. Malformed markers return ErrMalformedSection.
// The blank separator line ReplaceSection adds before an appended section is
// dropped too, so install followed by uninstall restores the original file.
func RemoveSection(existing string, m SectionMarkers) (out string, removed bool, err error) {
	span, err := m.locate(existing)
	if err != nil {
		return "", false, err
	}
	if span == nil {
		return existing, false, nil
	}
	before := existing[:span.from]
	after := existing[span.to:]
	if after == "" && strings.HasSuffix(before, "\n\n") {
		before = strings.TrimSuffix(before, "\n")
	}
	return before + after, true, nil
}

// WriteFileAtomic writes data to path via a temp file and a rename, keeping the
// existing file's permissions (0o644 for a new file), so an interrupted write
// never leaves a truncated file that holds user content.
func WriteFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	// Removing the temp file fails harmlessly once the rename has moved it.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() // the write error is the one worth reporting
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close() // the chmod error is the one worth reporting
		return fmt.Errorf("chmod temp for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// ErrMarkerInBody is returned when the body ReplaceSection would write contains
// the section's own start or end marker.
var ErrMarkerInBody = errors.New("section body contains a verikt section marker")
