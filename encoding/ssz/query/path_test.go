package query_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

// Helper to get pointer to uint64
func u64(v uint64) *uint64 { return &v }

func TestParsePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []query.PathElement
		wantErr  bool
	}{
		{
			name: "simple nested path",
			path: "data.target.root",
			expected: []query.PathElement{
				{Name: "data"},
				{Name: "target"},
				{Name: "root"},
			},
			wantErr: false,
		},
		{
			name: "simple nested path with leading dot",
			path: ".data.target.root",
			expected: []query.PathElement{
				{Name: "data"},
				{Name: "target"},
				{Name: "root"},
			},
			wantErr: false,
		},
		{
			name: "simple length path with length field",
			path: "data.target.len(root)",
			expected: []query.PathElement{
				{Name: "data"},
				{Name: "target"},
				{Name: "root", Length: true},
			},
			wantErr: false,
		},
		{
			name:     "len with top-level identifier",
			path:     "len(data)",
			expected: []query.PathElement{{Name: "data", Length: true}},
			wantErr:  false,
		},
		{
			name: "length with messy whitespace",
			path: "data.target. \tlen (  root  ) ",
			expected: []query.PathElement{
				{Name: "data"},
				{Name: "target"},
				{Name: "root", Length: true},
			},
			wantErr: false,
		},
		{
			name: "len with numeric index inside argument",
			path: "data.len(a[10])",
			expected: []query.PathElement{
				{Name: "data"},
				{Name: "a", Length: true, Index: u64(10)},
			},
			wantErr: false,
		},
		{
			name:     "array index with spaces",
			path:     "arr[  42 ]",
			expected: []query.PathElement{{Name: "arr", Index: u64(42)}},
			wantErr:  false,
		},
		{
			name:     "array leading zeros",
			path:     "arr[001]",
			expected: []query.PathElement{{Name: "arr", Index: u64(1)}},
			wantErr:  false,
		},
		{
			name:     "array max uint64",
			path:     "arr[18446744073709551615]",
			expected: []query.PathElement{{Name: "arr", Index: u64(18446744073709551615)}},
			wantErr:  false,
		},
		{
			name:     "len with dotted path inside - no input validation - reverts at a later stage",
			path:     "len(data.target.root)",
			expected: []query.PathElement{{Name: "len(data", Length: false}, {Name: "target", Length: false}, {Name: "root)", Length: false}},
			wantErr:  false,
		},
		{
			name:     "len with dotted path then more - no input validation - reverts at a later stage",
			path:     "len(data.target.root).foo",
			expected: []query.PathElement{{Name: "len(data", Length: false}, {Name: "target", Length: false}, {Name: "root)", Length: false}, {Name: "foo", Length: false}},
			wantErr:  false,
		},
		{
			name:     "len without closing paren - no input validation - reverts at a later stage",
			path:     "len(root",
			expected: []query.PathElement{{Name: "len(root"}},
			wantErr:  false,
		},
		{
			name:     "len with extra closing paren - no input validation - reverts at a later stage",
			path:     "len(root))",
			expected: []query.PathElement{{Name: "len(root))"}},
			wantErr:  false,
		},
		{
			name:     "empty len argument - no input validation - reverts at a later stage",
			path:     "len()",
			expected: []query.PathElement{{Name: "len()"}},
			wantErr:  false,
		},
		{
			name:     "len with comma-separated args - no input validation - reverts at a later stage",
			path:     "len(a,b)",
			expected: []query.PathElement{{Name: "a,b", Length: true}},
			wantErr:  false,
		},
		{
			name: "len call followed by index (outer) - no input validation - reverts at a later stage",
			path: "data.len(root)[0]",
			expected: []query.PathElement{
				{Name: "data"},
				{Name: "len(root)", Index: u64(0)},
			},
			wantErr: false,
		},
		{
			name:    "cannot provide consecutive dots in raw path",
			path:    "data..target.root",
			wantErr: true,
		},
		{
			name:    "cannot provide a negative index in array path",
			path:    ".data.target.root[-1]",
			wantErr: true,
		},
		{
			name:    "invalid index in array path",
			path:    ".data.target.root[a]",
			wantErr: true,
		},
		{
			name:    "multidimensional array index in path",
			path:    ".data.target.root[0][1]",
			wantErr: true,
		},
		{
			name:     "leading double dot",
			path:     "..data",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "trailing dot",
			path:     "data.target.",
			expected: nil,
			wantErr:  true,
		},
		{
			name:    "len with inner bracket non-numeric index",
			path:    "data.len(a[b])",
			wantErr: true,
		},
		{
			name:    "array empty index",
			path:    "arr[]",
			wantErr: true,
		},
		{
			name:    "array hex index",
			path:    "arr[0x10]",
			wantErr: true,
		},
		{
			name:    "array missing closing bracket",
			path:    "arr[12",
			wantErr: true,
		},
		{
			name:    "array plus sign index",
			path:    "arr[+3]",
			wantErr: true,
		},
		{
			name:    "array unicode digits",
			path:    "arr[１２]",
			wantErr: true,
		},
		{
			name:    "array overflow uint64",
			path:    "arr[18446744073709551616]",
			wantErr: true,
		},
		{
			name:     "array index then suffix",
			path:     "field[1]suffix",
			expected: []query.PathElement{{Name: "field", Index: u64(1)}},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedPath, err := query.ParsePath(tt.path)

			if tt.wantErr {
				require.NotNil(t, err, "Expected error did not occur")
				return
			}

			require.NoError(t, err)
			require.Equal(t, len(tt.expected), len(parsedPath), "Expected %d path elements, got %d", len(tt.expected), len(parsedPath))
			require.DeepEqual(t, tt.expected, parsedPath, "Parsed path does not match expected path")
		})
	}
}
