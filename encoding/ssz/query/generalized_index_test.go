package query_test

import (
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz/query"
	sszquerypb "github.com/OffchainLabs/prysm/v7/proto/ssz_query/testing"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestGetIndicesFromPath_FixedNestedContainer(t *testing.T) {
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
			name:         "Value3 field",
			path:         ".value3",
			expectError:  true,
			errorMessage: "field value3 not found",
		},
		{
			name:         "Basic field cannot descend",
			path:         "value1.value1",
			expectError:  true,
			errorMessage: "indexing requires a container field step first, got Uint64",
		},
		{
			name:         "Indexing without container step",
			path:         "value2.value2[0]",
			expectError:  true,
			errorMessage: "indexing requires a container field step first",
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
			expectedIndex: 3,
			expectError:   false,
		},
		{
			name:          "Value2 -> element[31]",
			path:          "value2[31]",
			expectedIndex: 3,
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

func TestGetIndicesFromPath_VariableTestContainer(t *testing.T) {
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
			expectedIndex: 16,
			expectError:   false,
		},
		{
			name:          "field_list_uint64",
			path:          "field_list_uint64",
			expectedIndex: 17,
			expectError:   false,
		},
		{
			name:          "len(field_list_uint64)",
			path:          "len(field_list_uint64)",
			expectedIndex: 35,
			expectError:   false,
		},
		{
			name:          "field_list_uint64[0]",
			path:          "field_list_uint64[0]",
			expectedIndex: 17408,
			expectError:   false,
		},
		{
			name:          "field_list_uint64[2047]",
			path:          "field_list_uint64[2047]",
			expectedIndex: 17919,
			expectError:   false,
		},
		{
			name:          "bitlist_field",
			path:          "bitlist_field",
			expectedIndex: 22,
			expectError:   false,
		},
		{
			name:          "bitlist_field[0]",
			path:          "bitlist_field[0]",
			expectedIndex: 352,
			expectError:   false,
		},
		{
			name:          "bitlist_field[1]",
			path:          "bitlist_field[1]",
			expectedIndex: 352,
			expectError:   false,
		},
		{
			name:          "len(bitlist_field)",
			path:          "len(bitlist_field)",
			expectedIndex: 45,
			expectError:   false,
		},
		{
			name:         "len(trailing_field)",
			path:         "len(trailing_field)",
			expectError:  true,
			errorMessage: "len() is only supported for List and Bitlist types, got Vector",
		},
		{
			name:          "field_list_container[0]",
			path:          "field_list_container[0]",
			expectedIndex: 4608,
			expectError:   false,
		},
		{
			name:          "nested",
			path:          "nested",
			expectedIndex: 20,
			expectError:   false,
		},
		{
			name:          "nested.field_list_uint64[10]",
			path:          "nested.field_list_uint64[10]",
			expectedIndex: 5186,
			expectError:   false,
		},
		{
			name:          "variable_container_list",
			path:          "variable_container_list",
			expectedIndex: 21,
			expectError:   false,
		},
		{
			name:          "len(variable_container_list)",
			path:          "len(variable_container_list)",
			expectedIndex: 43,
			expectError:   false,
		},
		{
			name:          "variable_container_list[0]",
			path:          "variable_container_list[0]",
			expectedIndex: 672,
			expectError:   false,
		},
		{
			name:          "variable_container_list[0].inner_1",
			path:          "variable_container_list[0].inner_1",
			expectedIndex: 1344,
			expectError:   false,
		},
		{
			name:          "variable_container_list[0].inner_1.field_list_uint64[1]",
			path:          "variable_container_list[0].inner_1.field_list_uint64[1]",
			expectedIndex: 344128,
			expectError:   false,
		},
		{
			name:         "len(variable_container_list[0].inner_1.nested_list_field[3])",
			path:         "len(variable_container_list[0].inner_1.nested_list_field[3])",
			expectError:  true,
			errorMessage: "length calculation error: len() is not supported for multi-dimensional arrays",
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

func TestGetIndicesFromPath_FixedTestContainer(t *testing.T) {
	testSpec := &sszquerypb.FixedTestContainer{}
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
			name:          "field_uint32",
			path:          "field_uint32",
			expectedIndex: 16,
			expectError:   false,
		},
		{
			name:          ".field_uint64",
			path:          ".field_uint64",
			expectedIndex: 17,
			expectError:   false,
		},
		{
			name:          "field_bool",
			path:          "field_bool",
			expectedIndex: 18,
			expectError:   false,
		},
		{
			name:          "field_bytes32",
			path:          "field_bytes32",
			expectedIndex: 19,
			expectError:   false,
		},
		{
			name:          "nested",
			path:          "nested",
			expectedIndex: 20,
			expectError:   false,
		},
		{
			name:          "vector_field",
			path:          "vector_field",
			expectedIndex: 21,
			expectError:   false,
		},
		{
			name:          "two_dimension_bytes_field",
			path:          "two_dimension_bytes_field",
			expectedIndex: 22,
			expectError:   false,
		},
		{
			name:          "bitvector64_field",
			path:          "bitvector64_field",
			expectedIndex: 23,
			expectError:   false,
		},
		{
			name:          "bitvector512_field",
			path:          "bitvector512_field",
			expectedIndex: 24,
			expectError:   false,
		},
		{
			name:          "bitvector64_field[0]",
			path:          "bitvector64_field[0]",
			expectedIndex: 23,
			expectError:   false,
		},
		{
			name:          "bitvector64_field[63]",
			path:          "bitvector64_field[63]",
			expectedIndex: 23,
			expectError:   false,
		},
		{
			name:          "bitvector512_field[0]",
			path:          "bitvector512_field[0]",
			expectedIndex: 48,
			expectError:   false,
		},
		{
			name:          "bitvector512_field[511]",
			path:          "bitvector512_field[511]",
			expectedIndex: 49,
			expectError:   false,
		},
		{
			name:          "trailing_field",
			path:          "trailing_field",
			expectedIndex: 25,
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
