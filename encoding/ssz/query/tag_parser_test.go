package query_test

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestParseSSZTag(t *testing.T) {
	tests := []struct {
		wantErr          bool
		wantIsList       bool
		wantIsVector     bool
		wantListLimit    uint64
		wantVectorLength uint64
		wantRemainingTag string
		tag              string
		name             string
	}{
		// Vector tests
		{
			name:             "single dimension vector",
			tag:              `ssz-size:"32"`,
			wantIsVector:     true,
			wantVectorLength: 32,
		},
		{
			name:             "multi-dimensional vector",
			tag:              `ssz-size:"5,32"`,
			wantIsVector:     true,
			wantVectorLength: 5,
			wantRemainingTag: `ssz-size:"32"`,
		},
		{
			name:             "three-dimensional vector",
			tag:              `ssz-size:"5,10,32"`,
			wantIsVector:     true,
			wantVectorLength: 5,
			wantRemainingTag: `ssz-size:"10,32"`,
		},
		{
			name:             "large vector",
			tag:              `ssz-size:"1048576"`,
			wantIsVector:     true,
			wantVectorLength: 1048576,
		},

		// List tests
		{
			name:          "single dimension list",
			tag:           `ssz-max:"100"`,
			wantIsList:    true,
			wantListLimit: 100,
		},
		{
			name:             "multi-dimensional list",
			tag:              `ssz-max:"100,200"`,
			wantIsList:       true,
			wantListLimit:    100,
			wantRemainingTag: `ssz-max:"200"`,
		},
		{
			name:          "large list",
			tag:           `ssz-max:"1048576"`,
			wantIsList:    true,
			wantListLimit: 1048576,
		},
		{
			name:          "wildcard size becomes list",
			tag:           `ssz-size:"?" ssz-max:"100"`,
			wantIsList:    true,
			wantListLimit: 100,
		},
		{
			name:             "wildcard with remaining dimensions",
			tag:              `ssz-size:"?,32" ssz-max:"100"`,
			wantIsList:       true,
			wantListLimit:    100,
			wantRemainingTag: `ssz-size:"32"`,
		},
		{
			name:          "empty size becomes list",
			tag:           `ssz-size:"" ssz-max:"100"`,
			wantIsList:    true,
			wantListLimit: 100,
		},
		{
			name:             "list of vectors",
			tag:              `ssz-size:"?,32" ssz-max:"100"`,
			wantIsList:       true,
			wantListLimit:    100,
			wantRemainingTag: `ssz-size:"32"`,
		},

		// Error cases
		{
			name:    "empty tag",
			tag:     "",
			wantErr: true,
		},
		{
			name:    "zero vector length",
			tag:     `ssz-size:"0"`,
			wantErr: true,
		},
		{
			name:    "zero list limit",
			tag:     `ssz-max:"0"`,
			wantErr: true,
		},
		{
			name:    "invalid vector length",
			tag:     `ssz-size:"abc"`,
			wantErr: true,
		},
		{
			name:    "invalid list limit",
			tag:     `ssz-max:"xyz"`,
			wantErr: true,
		},
		{
			name:    "both wildcard",
			tag:     `ssz-size:"?" ssz-max:"?"`,
			wantErr: true,
		},
		{
			name:    "list without max",
			tag:     `ssz-size:"?"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tag *reflect.StructTag
			if tt.tag != "" {
				structTag := reflect.StructTag(tt.tag)
				tag = &structTag
			}

			dim, remainingTag, err := query.ParseSSZTag(tag)
			if tt.wantErr {
				require.NotNil(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, dim)

			// Check dimension type
			require.Equal(t, tt.wantIsVector, dim.IsVector())
			require.Equal(t, tt.wantIsList, dim.IsList())

			// Verify vector length if it's a vector
			if tt.wantIsVector {
				length, err := dim.GetVectorLength()
				require.NoError(t, err)
				require.Equal(t, tt.wantVectorLength, length)

				// Trying to get list limit should error
				_, err = dim.GetListLimit()
				require.NotNil(t, err)
			}

			// Verify list limit if it's a list
			if tt.wantIsList {
				limit, err := dim.GetListLimit()
				require.NoError(t, err)
				require.Equal(t, tt.wantListLimit, limit)

				// Trying to get vector length should error
				_, err = dim.GetVectorLength()
				require.NotNil(t, err)
			}

			// Check remaining tag
			if tt.wantRemainingTag == "" {
				require.Equal(t, remainingTag == nil, true)
			} else {
				require.NotNil(t, remainingTag)
				require.Equal(t, tt.wantRemainingTag, string(*remainingTag))
			}
		})
	}
}
