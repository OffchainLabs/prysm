package query_test

import (
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	sszquerypb "github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestGetIndicesFromPath(t *testing.T) {
	fixedNestedContainer := &sszquerypb.FixedNestedContainer{}

	info, err := query.AnalyzeObject(fixedNestedContainer)
	require.NoError(t, err)
	require.NotNil(t, info, "Expected non-nil SSZ info")

	testCases := []struct {
		name          string
		path          string
		expectedIndex uint64
		expectError   bool
		errorMessage  string
	}{
		{
			name:          "Value1 field",
			path:          ".value1",
			expectedIndex: 2,
			expectError:   false,
		},
		{
			name:          "Value2 field",
			path:          "value2",
			expectedIndex: 3,
			expectError:   false,
		},
		{
			name:          "Value2 -> element[0]",
			path:          "value2[0]",
			expectedIndex: 96,
			expectError:   false,
		},
		{
			name:          "Value2 -> element[31]",
			path:          "value2[31]",
			expectedIndex: 127,
			expectError:   false,
		},
		{
			name:          "Empty path error",
			path:          "",
			expectedIndex: 0,
			expectError:   true,
			errorMessage:  "empty path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provingFields, err := query.ParsePath(tc.path)
			require.NoError(t, err)

			actualIndex, err := query.GetGeneralizedIndexFromPath(info, provingFields)

			if tc.expectError {
				require.NotNil(t, err)
				if tc.errorMessage != "" {
					if !strings.Contains(err.Error(), tc.errorMessage) {
						t.Errorf("Expected error message to contain '%s', but got: %s", tc.errorMessage, err.Error())
					}
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedIndex, actualIndex, "Generalized index mismatch for path: %s", tc.path)
				t.Logf("Path: %s -> Generalized Index: %v", tc.path, actualIndex)
			}
		})
	}
}

func TestGetIndicesFromPath_VariableNestedContainer(t *testing.T) {
	testSpec := &sszquerypb.VariableTestContainer{}
	info, err := query.AnalyzeObject(testSpec)
	require.NoError(t, err)
	require.NotNil(t, info, "Expected non-nil SSZ info")

	testCases := []struct {
		name          string
		path          string
		expectedIndex uint64
		expectError   bool
		errorMessage  string
	}{
		{
			name:          "leading_field",
			path:          "leading_field",
			expectedIndex: 8,
			expectError:   false,
		},
		{
			name:          ".leading_field",
			path:          ".leading_field",
			expectedIndex: 8,
			expectError:   false,
		},
		{
			name:          "field_list_uint64",
			path:          "field_list_uint64",
			expectedIndex: 9,
			expectError:   false,
		},
		{
			name:          "len(field_list_uint64)",
			path:          "len(field_list_uint64)",
			expectedIndex: 19,
			expectError:   false,
		},
		{
			name:          "field_list_uint64[0]",
			path:          "field_list_uint64[0]",
			expectedIndex: 9216,
			expectError:   false,
		},
		{
			name:          "field_list_uint64[2047]",
			path:          "field_list_uint64[2047]",
			expectedIndex: 9727,
			expectError:   false,
		},
		{
			name:          "nested",
			path:          "nested",
			expectedIndex: 12,
			expectError:   false,
		},
		{
			name:          "nested.field_list_uint64[10]",
			path:          "nested.field_list_uint64[10]",
			expectedIndex: 3138,
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provingFields, err := query.ParsePath(tc.path)
			require.NoError(t, err)

			actualIndex, err := query.GetGeneralizedIndexFromPath(info, provingFields)

			if tc.expectError {
				require.NotNil(t, err)
				if tc.errorMessage != "" {
					if !strings.Contains(err.Error(), tc.errorMessage) {
						t.Errorf("Expected error message to contain '%s', but got: %s", tc.errorMessage, err.Error())
					}
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedIndex, actualIndex, "Generalized index mismatch for path: %s", tc.path)
				t.Logf("Path: %s -> Generalized Index: %v", tc.path, actualIndex)
			}
		})
	}
}
