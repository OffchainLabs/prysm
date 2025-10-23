package query_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

var index = uint64(0)

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
