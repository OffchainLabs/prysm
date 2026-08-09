package query

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestListInfo_ElementValue(t *testing.T) {
	tests := []struct {
		name          string
		li            *listInfo
		index         int
		expectedValue any
		wantErr       bool
		errSubstring  string
	}{
		{
			name: "Success - valid slice index",
			li: &listInfo{
				sliceValue: reflect.ValueOf([]uint64{10, 20, 30}),
			},
			index:         1,
			expectedValue: uint64(20),
			wantErr:       false,
		},
		{
			name:         "Error - nil listInfo",
			li:           nil,
			index:        0,
			wantErr:      true,
			errSubstring: "listInfo is nil",
		},
		{
			name: "Error - invalid sliceValue (zero Value)",
			li: &listInfo{
				sliceValue: reflect.Value{},
			},
			index:        0,
			wantErr:      true,
			errSubstring: "sliceValue is not valid",
		},
		{
			name: "Error - incorrect kind (Map instead of Slice)",
			li: &listInfo{
				sliceValue: reflect.ValueOf(map[string]int{"a": 1}),
			},
			index:        0,
			wantErr:      true,
			errSubstring: "expected slice or array",
		},
		{
			name: "Error - incorrect kind (Int instead of Slice)",
			li: &listInfo{
				sliceValue: reflect.ValueOf(5),
			},
			index:        0,
			wantErr:      true,
			errSubstring: "expected slice or array",
		},
		{
			name: "Error - index out of bounds (negative)",
			li: &listInfo{
				sliceValue: reflect.ValueOf([]int{1, 2}),
			},
			index:        -1,
			wantErr:      true,
			errSubstring: "out of bounds",
		},
		{
			name: "Error - index out of bounds (too high)",
			li: &listInfo{
				sliceValue: reflect.ValueOf([]int{1, 2}),
			},
			index:        2,
			wantErr:      true,
			errSubstring: "out of bounds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.li.ElementValue(tt.index)
			if tt.wantErr {
				require.NotNil(t, err)
				if tt.errSubstring != "" {
					require.ErrorContains(t, tt.errSubstring, err)
				}
				return
			}

			require.Equal(t, true, got.IsValid())
			require.DeepEqual(t, tt.expectedValue, got.Interface())
		})
	}
}
