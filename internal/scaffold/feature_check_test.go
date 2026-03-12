package scaffold

import (
	"strings"
	"testing"
)

func TestCheckRequiredFeatures_AllPresent(t *testing.T) {
	resolved := map[string]bool{"slices_package": true, "os_root": true}
	matrix := &FeatureMatrix{
		Features: []Feature{
			{Name: "slices_package", Since: "1.21"},
			{Name: "os_root", Since: "1.24"},
		},
	}

	err := CheckRequiredFeatures([]string{"slices_package", "os_root"}, resolved, matrix, "1.24")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheckRequiredFeatures_SomeMissing(t *testing.T) {
	resolved := map[string]bool{"slices_package": true}
	matrix := &FeatureMatrix{
		Features: []Feature{
			{Name: "slices_package", Since: "1.21"},
			{Name: "os_root", Since: "1.24"},
		},
	}

	err := CheckRequiredFeatures([]string{"slices_package", "os_root"}, resolved, matrix, "1.20")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "os_root") {
		t.Errorf("error should mention os_root, got: %s", msg)
	}
	if !strings.Contains(msg, "1.24+") {
		t.Errorf("error should mention 1.24+, got: %s", msg)
	}
	if !strings.Contains(msg, "1.20") {
		t.Errorf("error should mention detected version 1.20, got: %s", msg)
	}
	if strings.Contains(msg, "slices_package") {
		t.Errorf("error should NOT mention slices_package (it's resolved), got: %s", msg)
	}
}

func TestCheckRequiredFeatures_EmptyRequired(t *testing.T) {
	err := CheckRequiredFeatures(nil, nil, nil, "1.20")
	if err != nil {
		t.Fatalf("expected nil for empty required, got %v", err)
	}

	err = CheckRequiredFeatures([]string{}, nil, nil, "1.20")
	if err != nil {
		t.Fatalf("expected nil for empty slice, got %v", err)
	}
}

func TestCheckRequiredFeatures_NilResolved(t *testing.T) {
	matrix := &FeatureMatrix{
		Features: []Feature{
			{Name: "slices_package", Since: "1.21"},
		},
	}

	err := CheckRequiredFeatures([]string{"slices_package"}, nil, matrix, "1.18")
	if err == nil {
		t.Fatal("expected error with nil resolved map, got nil")
	}
	if !strings.Contains(err.Error(), "slices_package") {
		t.Errorf("error should mention slices_package, got: %s", err.Error())
	}
}

func TestCheckRequiredFeatures_FeatureNotInMatrix(t *testing.T) {
	matrix := &FeatureMatrix{
		Features: []Feature{
			{Name: "slices_package", Since: "1.21"},
		},
	}

	err := CheckRequiredFeatures([]string{"nonexistent_feature"}, map[string]bool{}, matrix, "1.20")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown+") {
		t.Errorf("error should show unknown version for missing matrix entry, got: %s", err.Error())
	}
}
