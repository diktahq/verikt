package checker

import "testing"

// Writing to a nil map panics at runtime. `var m map[K]V` allocates nothing, so
// the write has to be preceded by make() or a literal.
func TestDetectNilMapWrite(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			"declared without make, then written",
			`package foo
func f() {
	var m map[string]int
	m["k"] = 1
}`,
			1,
		},
		{
			"package-level map written from a function",
			`package foo
var registry map[string]int
func register() { registry["a"] = 1 }`,
			1,
		},
		{
			"make before the write is safe",
			`package foo
func f() {
	var m map[string]int
	m = make(map[string]int)
	m["k"] = 1
}`,
			0,
		},
		{
			"composite literal is safe",
			`package foo
func f() {
	m := map[string]int{}
	m["k"] = 1
}`,
			0,
		},
		{
			"declared with make in the same statement is safe",
			`package foo
func f() {
	var m = make(map[string]int)
	m["k"] = 1
}`,
			0,
		},
		{
			"reading a nil map is legal and must not be flagged",
			`package foo
func f() int {
	var m map[string]int
	return m["k"]
}`,
			0,
		},
		{
			"slice index write is not a map",
			`package foo
func f() {
	var s []int
	s[0] = 1
}`,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, fset := parseSource(t, tt.src)
			got := detectNilMapWrite(file, fset, "test.go")
			if len(got) != tt.wantN {
				t.Fatalf("detectNilMapWrite() = %d findings, want %d: %v", len(got), tt.wantN, got)
			}
			for _, f := range got {
				if f.Name != "nil_map_write" || f.Severity != "error" {
					t.Errorf("unexpected finding shape: %+v", f)
				}
			}
		})
	}
}

// A single-value type assertion panics when the value is a different type. The
// two-value form and type switches are the safe alternatives.
func TestDetectTypeAssertionWithoutOK(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			"single-value assertion",
			`package foo
func f(v any) string { return v.(string) }`,
			1,
		},
		{
			"two-value form is safe",
			`package foo
func f(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}`,
			0,
		},
		{
			"type switch is safe",
			`package foo
func f(v any) string {
	switch t := v.(type) {
	case string:
		return t
	}
	return ""
}`,
			0,
		},
		{
			"assertion as a call argument",
			`package foo
func g(s string) {}
func f(v any) { g(v.(string)) }`,
			1,
		},
		{
			"two-value assignment to existing variables is safe",
			`package foo
func f(v any) {
	var s string
	var ok bool
	s, ok = v.(string)
	_, _ = s, ok
}`,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, fset := parseSource(t, tt.src)
			got := detectTypeAssertionWithoutOK(file, fset, "test.go")
			if len(got) != tt.wantN {
				t.Fatalf("detectTypeAssertionWithoutOK() = %d findings, want %d: %v", len(got), tt.wantN, got)
			}
			for _, f := range got {
				if f.Name != "type_assertion_without_ok" || f.Severity != "warning" {
					t.Errorf("unexpected finding shape: %+v", f)
				}
			}
		})
	}
}
