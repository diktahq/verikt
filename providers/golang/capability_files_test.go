package golang

import (
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"
)

// TestCapabilityFilesDoNotOverwriteEachOther: two capabilities that write the
// same file cannot both be selected; whichever renders last replaces the
// other's file. ddd and event-bus both wrote domain/event.go, so
// --cap ddd,event-bus lost DomainEvent and the project did not compile.
func TestCapabilityFilesDoNotOverwriteEachOther(t *testing.T) {
	writers := map[string][]string{} // template path inside files/ -> capabilities
	root := "templates/capabilities"
	err := fs.WalkDir(templatesFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		capName, rest, _ := strings.Cut(strings.TrimPrefix(p, root+"/"), "/")
		rel, ok := strings.CutPrefix(rest, "files/")
		if !ok {
			return nil
		}
		writers[rel] = append(writers[rel], capName)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rels := make([]string, 0, len(writers))
	for rel := range writers {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		if caps := writers[rel]; len(caps) > 1 {
			t.Errorf("%s is written by %s; selecting them together keeps only one",
				path.Clean(rel), strings.Join(caps, " and "))
		}
	}
}
