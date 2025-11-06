package query_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// Helper to get pointer to uint64
func u64(v uint64) *uint64 { return &v }

func TestParsePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected query.Path
		wantErr  bool
	}{
		{
			name: "simple path",
			path: "data",
			expected: query.Path{
				Length: false,
				Elements: []query.PathElement{
					{Name: "data"},
				},
			},
			wantErr: false,
		},
		{
			name: "simple path beginning with dot",
			path: ".data",
			expected: query.Path{
				Length: false,
				Elements: []query.PathElement{
					{Name: "data"},
				},
			},
			wantErr: false,
		},
		{
			name:    "simple path trailing dot",
			path:    "data.",
			wantErr: true,
		},
		{
			name:    "simple path surrounded by dot",
			path:    ".data.",
			wantErr: true,
		},
		{
			name:    "simple path beginning with two dots",
			path:    "..data",
			wantErr: true,
		},
		{
			name: "simple nested path",
			path: "data.target.root",
			expected: query.Path{
				Length: false,
				Elements: []query.PathElement{
					{Name: "data"},
					{Name: "target"},
					{Name: "root"},
				},
			},
			wantErr: false,
		},
		{
			name: "len with top-level identifier",
			path: "len(data)",
			expected: query.Path{
				Length: true,
				Elements: []query.PathElement{
					{Name: "data"},
				},
			},
			wantErr: false,
		},
		{
			name: "len with top-level identifier and leading dot",
			path: "len(.data)",
			expected: query.Path{
				Length: true,
				Elements: []query.PathElement{
					{Name: "data"},
				},
			},
			wantErr: false,
		},
		{
			name:    "len with top-level identifier and trailing dot",
			path:    "len(data.)",
			wantErr: true,
		},
		{
			name:    "len with top-level identifier beginning dot",
			path:    ".len(data)",
			wantErr: true,
		},
		{
			name: "len with dotted path inside",
			path: "len(data.target.root)",
			expected: query.Path{
				Length: true,
				Elements: []query.PathElement{
					{Name: "data"},
					{Name: "target"},
					{Name: "root"},
				},
			},
			wantErr: false,
		},
		{
			name:    "simple length path with non-outer length field",
			path:    "data.target.len(root)",
			wantErr: true,
		},
		{
			name: "simple path with `len` used as a field name",
			path: "data.len",
			expected: query.Path{
				Length: false,
				Elements: []query.PathElement{
					{Name: "data"},
					{Name: "len"},
				},
			},
			wantErr: false,
		},
		{
			name: "simple path with `len` used as a field name + trailing field",
			path: "data.len.value",
			expected: query.Path{
				Length: false,
				Elements: []query.PathElement{
					{Name: "data"},
					{Name: "len"},
					{Name: "value"},
				},
			},
			wantErr: false,
		},
		{
			name: "simple path with `len`",
			path: "len.len",
			expected: query.Path{
				Length: false,
				Elements: []query.PathElement{
					{Name: "len"},
					{Name: "len"},
				},
			},
			wantErr: false,
		},
		{
			name:    "simple length path with length field",
			path:    "len.len(root)",
			wantErr: true,
		},
		{
			name:    "empty length field",
			path:    "len()",
			wantErr: true,
		},
		{
			name:    "length field not terminal",
			path:    "len(data).foo",
			wantErr: true,
		},
		{
			name:    "length field with missing closing paren",
			path:    "len(data",
			wantErr: true,
		},
		{
			name:    "length field with two closing paren",
			path:    "len(data))",
			wantErr: true,
		},
		{
			name:    "len with comma-separated args",
			path:    "len(a,b)",
			wantErr: true,
		},
		{
			name: "array index path",
			path: "arr[42]",
			expected: query.Path{
				Length: false,
				Elements: []query.PathElement{
					{Name: "arr", Index: u64(42)},
				},
			},
			wantErr: false,
		},
		{
			name: "array index path with max uint64",
			path: "arr[18446744073709551615]",
			expected: query.Path{
				Length: false,
				Elements: []query.PathElement{
					{Name: "arr", Index: u64(18446744073709551615)},
				},
			},
			wantErr: false,
		},
		{
			name:    "array element in wrong nested path",
			path:    "arr[42]foo",
			wantErr: true,
		},
		{
			name: "array index in nested path",
			path: "arr[42].foo",
			expected: query.Path{
				Length: false,
				Elements: []query.PathElement{
					{Name: "arr", Index: u64(42)},
					{Name: "foo"},
				},
			},
			wantErr: false,
		},
		{
			name: "array index in deeper nested path",
			path: "arr[42].foo.bar[10]",
			expected: query.Path{
				Length: false,
				Elements: []query.PathElement{
					{Name: "arr", Index: u64(42)},
					{Name: "foo"},
					{Name: "bar", Index: u64(10)},
				},
			},
			wantErr: false,
		},
		{
			name: "length of array element",
			path: "len(arr[42])",
			expected: query.Path{
				Length: true,
				Elements: []query.PathElement{
					{Name: "arr", Index: u64(42)},
				},
			},
			wantErr: false,
		},
		{
			name:    "length of array + trailing item",
			path:    "len(arr)[0]",
			wantErr: true,
		},
		{
			name: "length of nested path within array element",
			path: "len(arr[42].foo)",
			expected: query.Path{
				Length: true,
				Elements: []query.PathElement{
					{Name: "arr", Index: u64(42)},
					{Name: "foo"},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty spaces in path",
			path:    "data . target",
			wantErr: true,
		},
		{
			name:    "leading dot +  empty spaces",
			path:    ". data",
			wantErr: true,
		},
		{
			name:    "length with leading dot +  empty spaces",
			path:    "len(. data)",
			wantErr: true,
		},
		{
			name:     "Empty path error",
			path:     "",
			expected: query.Path{},
		},
		{
			name:    "length with leading dot +  empty spaces",
			path:    "test))((",
			wantErr: true,
		},
		{
			name:    "length with leading dot +  empty spaces",
			path:    "array][0][",
			wantErr: true,
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
			require.Equal(t, len(tt.expected.Elements), len(parsedPath.Elements), "Expected %d path elements, got %d", len(tt.expected.Elements), len(parsedPath.Elements))
			require.DeepEqual(t, tt.expected, parsedPath, "Parsed path does not match expected path")
		})
	}
}
