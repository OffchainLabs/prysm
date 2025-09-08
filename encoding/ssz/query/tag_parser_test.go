package query_test

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestParseSSZTag(t *testing.T) {
	tests := []struct {
		name             string
		tag              string
		wantSizeValue    string
		wantMaxValue     string
		wantHasSize      bool
		wantHasMax       bool
		wantRemainingTag string
		wantErr          bool
	}{
		{
			name:             "single dimension vector",
			tag:              `ssz-size:"32"`,
			wantSizeValue:    "32",
			wantHasSize:      true,
			wantHasMax:       false,
			wantRemainingTag: "",
		},
		{
			name:             "multi-dimensional vector",
			tag:              `ssz-size:"5,32"`,
			wantSizeValue:    "5",
			wantHasSize:      true,
			wantHasMax:       false,
			wantRemainingTag: `ssz-size:"32"`,
		},
		{
			name:             "three-dimensional vector",
			tag:              `ssz-size:"5,10,32"`,
			wantSizeValue:    "5",
			wantHasSize:      true,
			wantHasMax:       false,
			wantRemainingTag: `ssz-size:"10,32"`,
		},
		{
			name:             "single dimension list",
			tag:              `ssz-max:"100"`,
			wantMaxValue:     "100",
			wantHasSize:      false,
			wantHasMax:       true,
			wantRemainingTag: "",
		},
		{
			name:             "multi-dimensional list",
			tag:              `ssz-max:"100,200"`,
			wantMaxValue:     "100",
			wantHasSize:      false,
			wantHasMax:       true,
			wantRemainingTag: `ssz-max:"200"`,
		},
		{
			name:             "mixed vector and list",
			tag:              `ssz-size:"5" ssz-max:"100"`,
			wantSizeValue:    "5",
			wantMaxValue:     "100",
			wantHasSize:      true,
			wantHasMax:       true,
			wantRemainingTag: "",
		},
		{
			name:             "mixed multi-dimensional",
			tag:              `ssz-size:"5,32" ssz-max:"100,200"`,
			wantSizeValue:    "5",
			wantMaxValue:     "100",
			wantHasSize:      true,
			wantHasMax:       true,
			wantRemainingTag: `ssz-size:"32" ssz-max:"200"`,
		},
		{
			name:             "wildcard for variable size",
			tag:              `ssz-size:"?,32" ssz-max:"100"`,
			wantSizeValue:    "?",
			wantMaxValue:     "100",
			wantHasSize:      true,
			wantHasMax:       true,
			wantRemainingTag: `ssz-size:"32"`,
		},
		{
			name:             "empty tag",
			tag:              "",
			wantHasSize:      false,
			wantHasMax:       false,
			wantRemainingTag: "",
		},
		{
			name:             "nil tag",
			tag:              "",
			wantHasSize:      false,
			wantHasMax:       false,
			wantRemainingTag: "",
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

			if tt.tag == "" {
				// For empty/nil tags, ParseSSZTag returns nil
				require.Equal(t, dim == nil, true)
				require.Equal(t, remainingTag == nil, true)
				return
			}

			require.NotNil(t, dim)
			require.Equal(t, tt.wantSizeValue, dim.SizeValue)
			require.Equal(t, tt.wantMaxValue, dim.MaxValue)
			require.Equal(t, tt.wantHasSize, dim.HasSize)
			require.Equal(t, tt.wantHasMax, dim.HasMax)

			if tt.wantRemainingTag == "" {
				require.Equal(t, remainingTag == nil, true)
			} else {
				require.NotNil(t, remainingTag)
				require.Equal(t, tt.wantRemainingTag, string(*remainingTag))
			}
		})
	}
}

func TestSSZDimension_IsVector(t *testing.T) {
	tests := []struct {
		name string
		dim  query.SSZDimension
		want bool
	}{
		{
			name: "valid vector",
			dim:  query.SSZDimension{SizeValue: "32", HasSize: true},
			want: true,
		},
		{
			name: "wildcard is not vector",
			dim:  query.SSZDimension{SizeValue: "?", HasSize: true},
			want: false,
		},
		{
			name: "empty size is not vector",
			dim:  query.SSZDimension{SizeValue: "", HasSize: true},
			want: false,
		},
		{
			name: "no size flag",
			dim:  query.SSZDimension{SizeValue: "32", HasSize: false},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.dim.IsVector())
		})
	}
}

func TestSSZDimension_IsList(t *testing.T) {
	tests := []struct {
		name string
		dim  query.SSZDimension
		want bool
	}{
		{
			name: "valid list with max only",
			dim:  query.SSZDimension{MaxValue: "100", HasMax: true},
			want: true,
		},
		{
			name: "list with wildcard size",
			dim:  query.SSZDimension{SizeValue: "?", MaxValue: "100", HasSize: true, HasMax: true},
			want: true,
		},
		{
			name: "list with empty size",
			dim:  query.SSZDimension{SizeValue: "", MaxValue: "100", HasSize: true, HasMax: true},
			want: true,
		},
		{
			name: "vector is not list",
			dim:  query.SSZDimension{SizeValue: "32", MaxValue: "100", HasSize: true, HasMax: true},
			want: false,
		},
		{
			name: "no max flag",
			dim:  query.SSZDimension{MaxValue: "100", HasMax: false},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.dim.IsList())
		})
	}
}

func TestSSZDimension_GetVectorLength(t *testing.T) {
	tests := []struct {
		name    string
		dim     query.SSZDimension
		want    uint64
		wantErr bool
	}{
		{
			name: "valid vector length",
			dim:  query.SSZDimension{SizeValue: "32", HasSize: true},
			want: 32,
		},
		{
			name:    "not a vector",
			dim:     query.SSZDimension{SizeValue: "?", HasSize: true},
			wantErr: true,
		},
		{
			name:    "invalid number",
			dim:     query.SSZDimension{SizeValue: "abc", HasSize: true},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.dim.GetVectorLength()
			if tt.wantErr {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSSZDimension_GetListLimit(t *testing.T) {
	tests := []struct {
		name    string
		dim     query.SSZDimension
		want    uint64
		wantErr bool
	}{
		{
			name: "valid list limit",
			dim:  query.SSZDimension{MaxValue: "100", HasMax: true},
			want: 100,
		},
		{
			name:    "not a list",
			dim:     query.SSZDimension{SizeValue: "32", HasSize: true},
			wantErr: true,
		},
		{
			name:    "invalid number",
			dim:     query.SSZDimension{MaxValue: "abc", HasMax: true},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.dim.GetListLimit()
			if tt.wantErr {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
