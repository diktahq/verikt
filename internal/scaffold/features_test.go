package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFeatureMatrix(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *FeatureMatrix
		wantErr string
	}{
		{
			name: "valid features",
			input: `features:
  - name: range-over-int
    description: "Range over integer values"
    since: "1.22"
  - name: generics
    description: "Type parameters"
    since: "1.18"
`,
			want: &FeatureMatrix{
				Features: []Feature{
					{Name: "range-over-int", Description: "Range over integer values", Since: "1.22"},
					{Name: "generics", Description: "Type parameters", Since: "1.18"},
				},
			},
		},
		{
			name:  "empty features list",
			input: "features: []\n",
			want:  &FeatureMatrix{Features: []Feature{}},
		},
		{
			name:  "no features key",
			input: "{}\n",
			want:  &FeatureMatrix{},
		},
		{
			name:    "feature missing name",
			input:   "features:\n  - description: test\n    since: \"1.0\"\n",
			wantErr: "feature missing name",
		},
		{
			name:    "feature missing since",
			input:   "features:\n  - name: foo\n    description: test\n",
			wantErr: "missing since version",
		},
		{
			name:    "invalid yaml",
			input:   "features: [[[",
			wantErr: "parse feature matrix",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseFeatureMatrix([]byte(tc.input))
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveFeatures(t *testing.T) {
	matrix := &FeatureMatrix{
		Features: []Feature{
			{Name: "generics", Since: "1.18"},
			{Name: "range-over-int", Since: "1.22"},
			{Name: "iterators", Since: "1.23"},
		},
	}

	tests := []struct {
		name     string
		version  string
		matrix   *FeatureMatrix
		expected map[string]bool
		wantErr  string
	}{
		{
			name:    "version enables all features",
			version: "1.24",
			matrix:  matrix,
			expected: map[string]bool{
				"generics":       true,
				"range-over-int": true,
				"iterators":      true,
			},
		},
		{
			name:    "version enables some features",
			version: "1.22",
			matrix:  matrix,
			expected: map[string]bool{
				"generics":       true,
				"range-over-int": true,
				"iterators":      false,
			},
		},
		{
			name:    "version enables no features",
			version: "1.17",
			matrix:  matrix,
			expected: map[string]bool{
				"generics":       false,
				"range-over-int": false,
				"iterators":      false,
			},
		},
		{
			name:    "exact version match",
			version: "1.18",
			matrix:  matrix,
			expected: map[string]bool{
				"generics":       true,
				"range-over-int": false,
				"iterators":      false,
			},
		},
		{
			name:     "nil matrix returns empty map",
			version:  "1.24",
			matrix:   nil,
			expected: map[string]bool{},
		},
		{
			name:    "invalid detected version",
			version: "abc",
			matrix:  matrix,
			wantErr: "parse detected version",
		},
		{
			name:    "invalid since version in feature",
			version: "1.24",
			matrix: &FeatureMatrix{
				Features: []Feature{{Name: "bad", Since: "x.y"}},
			},
			wantErr: "parse since version",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveFeatures(tc.version, tc.matrix)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestVersionComparison(t *testing.T) {
	tests := []struct {
		name     string
		detected string
		since    string
		want     bool
	}{
		{"1.24 >= 1.21", "1.24", "1.21", true},
		{"3.10 >= 3.11", "3.10", "3.11", false},
		{"8.1 >= 8.0", "8.1", "8.0", true},
		{"equal versions", "1.22", "1.22", true},
		{"major only detected", "2", "1.99", true},
		{"major only since", "1.5", "2", false},
		{"three segment versions", "1.22.1", "1.22.0", true},
		{"three vs two segments", "1.22.0", "1.22", true},
		{"three segment less", "1.21.9", "1.22.0", false},
		{"single digit equal", "5", "5", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matrix := &FeatureMatrix{
				Features: []Feature{{Name: "test", Since: tc.since}},
			}
			got, err := ResolveFeatures(tc.detected, matrix)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got["test"])
		})
	}
}
