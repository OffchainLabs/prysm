package query_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedPath, err := query.ParsePath(tt.path)

			if tt.wantErr {
				require.NotNil(t, err, "Expected error but got none")
				return
			}

			require.NoError(t, err)
			require.Equal(t, len(tt.expected), len(parsedPath), "Expected %d path elements, got %d", len(tt.expected), len(parsedPath))
			require.DeepEqual(t, tt.expected, parsedPath, "Parsed path does not match expected path")
		})
	}
}
