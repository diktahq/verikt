package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestDefaultVeriktConfigPerArchitecture guards issue #18: the default config
// for clean and layered must describe clean and layered, not hexagonal.
func TestDefaultVeriktConfigPerArchitecture(t *testing.T) {
	cases := []struct {
		arch         string
		components   []string
		requiredDirs []string
	}{
		{
			arch:         "hexagonal",
			components:   []string{"domain", "ports", "service", "adapters", "platform", "app"},
			requiredDirs: []string{"cmd/", "domain/", "port/", "adapter/"},
		},
		{
			arch:         "clean",
			components:   []string{"entity", "usecase", "interface", "infrastructure", "platform", "app"},
			requiredDirs: []string{"cmd/", "internal/entity/", "internal/usecase/", "internal/interface/", "internal/infrastructure/"},
		},
		{
			arch:         "layered",
			components:   []string{"handler", "service", "repository", "model", "platform", "app"},
			requiredDirs: []string{"cmd/", "internal/handler/", "internal/service/", "internal/repository/", "internal/model/"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.arch, func(t *testing.T) {
			cfg := DefaultVeriktConfig("go", tc.arch)
			if errs := ValidateVeriktYAML(cfg); len(errs) > 0 {
				t.Fatalf("default %s config is invalid: %v", tc.arch, errs)
			}
			var names []string
			for _, c := range cfg.Components {
				names = append(names, c.Name)
			}
			if !reflect.DeepEqual(names, tc.components) {
				t.Errorf("components = %v, want %v", names, tc.components)
			}
			if got := cfg.Rules.Structure.RequiredDirs; !reflect.DeepEqual(got, tc.requiredDirs) {
				t.Errorf("required_dirs = %v, want %v", got, tc.requiredDirs)
			}
			// The app component owns cmd/, so it is checked, not excluded.
			if len(cfg.Check.Exclude) != 0 {
				t.Errorf("check.exclude = %v, want none", cfg.Check.Exclude)
			}
		})
	}
}

// TestDefaultVeriktConfigOnlyPlatformIsOptional checks that capability-written
// platform code may be absent while every architecture layer, and the
// composition root, must exist.
func TestDefaultVeriktConfigOnlyPlatformIsOptional(t *testing.T) {
	for _, arch := range []string{"hexagonal", "clean", "layered"} {
		sawPlatform := false
		for _, c := range DefaultVeriktConfig("go", arch).Components {
			sawPlatform = sawPlatform || c.Name == "platform"
			if want := c.Name == "platform"; c.Optional != want {
				t.Errorf("%s: component %q optional = %v, want %v", arch, c.Name, c.Optional, want)
			}
		}
		if !sawPlatform {
			t.Errorf("%s: no platform component declared", arch)
		}
	}
}

func TestComponentOptionalLoadsFromYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verikt.yaml")
	data := []byte(`language: go
architecture: hexagonal
components:
  - name: domain
    in: ["domain/**"]
    may_depend_on: []
  - name: platform
    in: ["platform/**"]
    may_depend_on: [domain]
    optional: true
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadVeriktYAML(path)
	if err != nil {
		t.Fatalf("LoadVeriktYAML: %v", err)
	}
	if cfg.Components[0].Optional {
		t.Error("domain: optional = true, want false when the key is absent")
	}
	if !cfg.Components[1].Optional {
		t.Error("platform: optional = false, want true")
	}
}
