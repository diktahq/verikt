package scaffold

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func capFS(manifests map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range manifests {
		fsys["templates/capabilities/"+name+"/capability.yaml"] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func TestResolveCapabilityDeps_AddsTransitiveRequiresFirst(t *testing.T) {
	fsys := capFS(map[string]string{
		"a": "name: a\nrequires: [b]\n",
		"b": "name: b\nrequires: [c]\n",
		"c": "name: c\n",
	})
	capSet := map[string]bool{"a": true}

	got, err := resolveCapabilityDeps(fsys, []string{"a"}, capSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"c", "b", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !capSet["b"] || !capSet["c"] {
		t.Errorf("capSet not updated with resolved deps: %v", capSet)
	}
}

func TestResolveCapabilityDeps_RejectsUnknownRequirement(t *testing.T) {
	fsys := capFS(map[string]string{
		"s3": "name: s3\nrequires: [config]\n",
	})

	got, err := resolveCapabilityDeps(fsys, []string{"s3"}, map[string]bool{"s3": true})
	if err == nil {
		t.Fatalf("expected error, got result %v", got)
	}
	want := `capability "s3" requires "config", which is not a known capability`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestResolveCapabilityDeps_RejectsUnknownTransitiveRequirement(t *testing.T) {
	fsys := capFS(map[string]string{
		"a": "name: a\nrequires: [b]\n",
		"b": "name: b\nrequires: [ghost]\n",
	})

	_, err := resolveCapabilityDeps(fsys, []string{"a"}, map[string]bool{"a": true})
	if err == nil {
		t.Fatal("expected error for transitive unknown requirement")
	}
	if !strings.Contains(err.Error(), `"b" requires "ghost"`) {
		t.Errorf("error %q does not name the requiring capability and the missing one", err)
	}
}

func TestResolveCapabilityDeps_RejectsMalformedManifest(t *testing.T) {
	fsys := capFS(map[string]string{
		"a": "name: a\nrequires: [b]\n",
		"b": "name: [not a string\n",
	})

	_, err := resolveCapabilityDeps(fsys, []string{"a"}, map[string]bool{"a": true})
	if err == nil {
		t.Fatal("expected error for malformed manifest")
	}
	if !strings.Contains(err.Error(), `capability "b"`) {
		t.Errorf("error %q does not name the malformed capability", err)
	}
}

// Partials render in capability order, so a capability must come after every
// capability it requires, whatever order the user typed (--cap health,http-api
// rendered health's router.Get before http-api declared router).
func TestResolveCapabilityDeps_PutsSelectedRequirementsFirst(t *testing.T) {
	fsys := capFS(map[string]string{
		"health":    "name: health\nrequires: [http-api]\n",
		"http-api":  "name: http-api\n",
		"bootstrap": "name: bootstrap\nrequires: [platform]\n",
		"platform":  "name: platform\n",
	})
	user := []string{"health", "http-api", "bootstrap"}
	capSet := map[string]bool{"health": true, "http-api": true, "bootstrap": true}

	got, err := resolveCapabilityDeps(fsys, user, capSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"platform", "http-api", "health", "bootstrap"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveCapabilityDeps_KeepsUserOrderAmongIndependentCapabilities(t *testing.T) {
	fsys := capFS(map[string]string{
		"z": "name: z\n",
		"a": "name: a\n",
		"m": "name: m\n",
	})
	user := []string{"z", "a", "m"}

	got, err := resolveCapabilityDeps(fsys, user, map[string]bool{"z": true, "a": true, "m": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, user) {
		t.Errorf("got %v, want the user's order %v", got, user)
	}
}

func TestResolveCapabilityDeps_RejectsRequiresCycle(t *testing.T) {
	fsys := capFS(map[string]string{
		"a": "name: a\nrequires: [b]\n",
		"b": "name: b\nrequires: [c]\n",
		"c": "name: c\nrequires: [a]\n",
	})

	got, err := resolveCapabilityDeps(fsys, []string{"a"}, map[string]bool{"a": true})
	if err == nil {
		t.Fatalf("expected a cycle error, got %v", got)
	}
	want := "capability requires form a cycle: b -> c -> a -> b"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}
