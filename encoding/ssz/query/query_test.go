package query_test

import (
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query/testutil"
	sszquerypb "github.com/OffchainLabs/prysm/v6/proto/ssz_query/testing"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/prysmaticlabs/go-bitfield"
)

func TestSize(t *testing.T) {
	tests := []struct {
		name         string
		obj          query.SSZObject
		expectedSize uint64
	}{
		{
			name:         "FixedTestContainer",
			obj:          &sszquerypb.FixedTestContainer{},
			expectedSize: 565,
		},
		{
			name:         "VariableTestContainer",
			obj:          &sszquerypb.VariableTestContainer{},
			expectedSize: 132,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := query.AnalyzeObject(tt.obj)
			require.NoError(t, err)
			require.NotNil(t, info)
			require.Equal(t, tt.expectedSize, info.Size())
		})
	}
}

func TestCalculateOffsetAndLength(t *testing.T) {
	type testCase struct {
		name           string
		path           string
		expectedOffset uint64
		expectedLength uint64
	}

	t.Run("FixedTestContainer", func(t *testing.T) {
		tests := []testCase{
			// Basic integer types
			{
				name:           "field_uint32",
				path:           ".field_uint32",
				expectedOffset: 0,
				expectedLength: 4,
			},
			{
				name:           "field_uint64",
				path:           ".field_uint64",
				expectedOffset: 4,
				expectedLength: 8,
			},
			// Boolean type
			{
				name:           "field_bool",
				path:           ".field_bool",
				expectedOffset: 12,
				expectedLength: 1,
			},
			// Fixed-size bytes
			{
				name:           "field_bytes32",
				path:           ".field_bytes32",
				expectedOffset: 13,
				expectedLength: 32,
			},
			// Nested container
			{
				name:           "nested container",
				path:           ".nested",
				expectedOffset: 45,
				expectedLength: 40,
			},
			{
				name:           "nested value1",
				path:           ".nested.value1",
				expectedOffset: 45,
				expectedLength: 8,
			},
			{
				name:           "nested value2",
				path:           ".nested.value2",
				expectedOffset: 53,
				expectedLength: 32,
			},
			// Vector field
			{
				name:           "vector field",
				path:           ".vector_field",
				expectedOffset: 85,
				expectedLength: 192, // 24 * 8 bytes
			},
			// Accessing an element in the vector
			{
				name:           "vector field (0th element)",
				path:           ".vector_field[0]",
				expectedOffset: 85,
				expectedLength: 8,
			},
			{
				name:           "vector field (10th element)",
				path:           ".vector_field[10]",
				expectedOffset: 165,
				expectedLength: 8,
			},
			// 2D bytes field
			{
				name:           "two_dimension_bytes_field",
				path:           ".two_dimension_bytes_field",
				expectedOffset: 277,
				expectedLength: 160, // 5 * 32 bytes
			},
			// Accessing an element in the 2D bytes field
			{
				name:           "two_dimension_bytes_field (1st element)",
				path:           ".two_dimension_bytes_field[1]",
				expectedOffset: 309,
				expectedLength: 32,
			},
			// Bitvector fields
			{
				name:           "bitvector64_field",
				path:           ".bitvector64_field",
				expectedOffset: 437,
				expectedLength: 8,
			},
			{
				name:           "bitvector512_field",
				path:           ".bitvector512_field",
				expectedOffset: 445,
				expectedLength: 64,
			},
			// Trailing field
			{
				name:           "trailing_field",
				path:           ".trailing_field",
				expectedOffset: 509,
				expectedLength: 56,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				path, err := query.ParsePath(tt.path)
				require.NoError(t, err)

				info, err := query.AnalyzeObject(&sszquerypb.FixedTestContainer{})
				require.NoError(t, err)

				_, offset, length, err := query.CalculateOffsetAndLength(info, path)
				require.NoError(t, err)

				require.Equal(t, tt.expectedOffset, offset, "Expected offset to be %d", tt.expectedOffset)
				require.Equal(t, tt.expectedLength, length, "Expected length to be %d", tt.expectedLength)
			})
		}
	})

	t.Run("VariableTestContainer", func(t *testing.T) {
		tests := []testCase{
			// Fixed leading field
			{
				name:           "leading_field",
				path:           ".leading_field",
				expectedOffset: 0,
				expectedLength: 32,
			},
			// Variable-size list fields
			{
				name:           "field_list_uint64",
				path:           ".field_list_uint64",
				expectedOffset: 116, // First part of variable-sized type.
				expectedLength: 40,  // 5 elements * uint64 (8 bytes each)
			},
			// Accessing an element in the list
			{
				name:           "field_list_uint64 (2nd element)",
				path:           ".field_list_uint64[2]",
				expectedOffset: 132,
				expectedLength: 8,
			},
			{
				name:           "field_list_container",
				path:           ".field_list_container",
				expectedOffset: 156, // Second part of variable-sized type.
				expectedLength: 120, // 3 elements * FixedNestedContainer (40 bytes each)
			},
			// Accessing an element in the list of containers
			{
				name:           "field_list_container (1st element)",
				path:           ".field_list_container[1]",
				expectedOffset: 196,
				expectedLength: 40,
			},
			{
				name:           "field_list_bytes32",
				path:           ".field_list_bytes32",
				expectedOffset: 276,
				expectedLength: 96, // 3 elements * 32 bytes each
			},
			// Accessing an element in the list of bytes32
			{
				name:           "field_list_bytes32 (0th element)",
				path:           ".field_list_bytes32[0]",
				expectedOffset: 276,
				expectedLength: 32,
			},
			{
				name:           "field_list_bytes32 (2nd element)",
				path:           ".field_list_bytes32[2]",
				expectedOffset: 340,
				expectedLength: 32,
			},
			// Nested paths
			{
				name:           "nested",
				path:           ".nested",
				expectedOffset: 372,
				// Calculated with:
				// - Value1: 8 bytes
				// - field_list_uint64 offset: 4 bytes
				// - field_list_uint64 length: 40 bytes
				// - nested_list_field offset: 4 bytes
				// - nested_list_field length: 99 bytes
				// - 3 offset pointers for each element in nested_list_field: 12 bytes
				// Total: 8 + 4 + 40 + 4 + 99 + 12 = 167 bytes
				expectedLength: 167,
			},
			{
				name:           "nested.value1",
				path:           ".nested.value1",
				expectedOffset: 372,
				expectedLength: 8,
			},
			{
				name:           "nested.field_list_uint64",
				path:           ".nested.field_list_uint64",
				expectedOffset: 388,
				expectedLength: 40,
			},
			{
				name:           "nested.field_list_uint64 (3rd element)",
				path:           ".nested.field_list_uint64[3]",
				expectedOffset: 412,
				expectedLength: 8,
			},
			{
				name:           "nested.nested_list_field",
				path:           ".nested.nested_list_field",
				expectedOffset: 440,
				expectedLength: 99,
			},
			// Accessing an element in the nested list of bytes
			{
				name:           "nested.nested_list_field (1st element)",
				path:           ".nested.nested_list_field[1]",
				expectedOffset: 472,
				expectedLength: 33,
			},
			{
				name:           "nested.nested_list_field (2nd element)",
				path:           ".nested.nested_list_field[2]",
				expectedOffset: 505,
				expectedLength: 34,
			},
			// Variable list of variable-sized containers
			{
				name:           "variable_container_list",
				path:           ".variable_container_list",
				expectedOffset: 547,
				expectedLength: 604,
			},
			// Bitlist field
			{
				name:           "bitlist_field",
				path:           ".bitlist_field",
				expectedOffset: 1151,
				expectedLength: 33, // 32 bytes + 1 byte for length delimiter
			},
			// 2D bytes field
			{
				name:           "nested_list_field",
				path:           ".nested_list_field",
				expectedOffset: 1196,
				expectedLength: 99,
			},
			// Accessing an element in the list of nested bytes
			{
				name:           "nested_list_field (0th element)",
				path:           ".nested_list_field[0]",
				expectedOffset: 1196,
				expectedLength: 32,
			},
			{
				name:           "nested_list_field (1st element)",
				path:           ".nested_list_field[1]",
				expectedOffset: 1228,
				expectedLength: 33,
			},
			{
				name:           "nested_list_field (2nd element)",
				path:           ".nested_list_field[2]",
				expectedOffset: 1261,
				expectedLength: 34,
			},
			// Fixed trailing field
			{
				name:           "trailing_field",
				path:           ".trailing_field",
				expectedOffset: 60, // After leading_field + 7 offset pointers
				expectedLength: 56,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				path, err := query.ParsePath(tt.path)
				require.NoError(t, err)

				testContainer := createVariableTestContainer()

				info, err := query.AnalyzeObject(testContainer)
				require.NoError(t, err)

				_, offset, length, err := query.CalculateOffsetAndLength(info, path)
				require.NoError(t, err)

				require.Equal(t, tt.expectedOffset, offset, "Expected offset to be %d", tt.expectedOffset)
				require.Equal(t, tt.expectedLength, length, "Expected length to be %d", tt.expectedLength)
			})
		}
	})
}

func TestHashTreeRoot(t *testing.T) {
	tests := []struct {
		name string
		obj  query.SSZObject
	}{
		{
			name: "FixedNestedContainer",
			obj: &sszquerypb.FixedNestedContainer{
				Value1: 42,
				Value2: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
			},
		},
		{
			name: "FixedTestContainer",
			obj:  createFixedTestContainer(),
		},
		{
			name: "VariableNestedContainer",
			obj: &sszquerypb.VariableNestedContainer{
				Value1:          84,
				FieldListUint64: []uint64{1, 2, 3, 4, 5},
				NestedListField: [][]byte{
					{0x0a, 0x0b, 0x0c},
					{0x1a, 0x1b, 0x1c, 0x1d},
				},
			},
		},
		{
			name: "VariableTestContainer",
			obj:  createVariableTestContainer(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Analyze the object to get its sszInfo
			info, err := query.AnalyzeObject(tt.obj)
			require.NoError(t, err)
			require.NotNil(t, info, "Expected non-nil SSZ info")

			// Call HashTreeRoot on the sszInfo and compare results
			hashTreeRoot, err := info.HashTreeRoot()
			require.NoError(t, err, "HashTreeRoot should not return an error")
			expectedHashTreeRoot, err := tt.obj.HashTreeRoot()
			require.NoError(t, err, "HashTreeRoot on original object should not return an error")
			require.Equal(t, expectedHashTreeRoot, hashTreeRoot, "HashTreeRoot from sszInfo should match original object's HashTreeRoot")
		})
	}
}

func TestRoundTripSszInfo(t *testing.T) {
	specs := []testutil.TestSpec{
		getFixedTestContainerSpec(),
		getVariableTestContainerSpec(),
	}

	for _, spec := range specs {
		testutil.RunStructTest(t, spec)
	}
}

func createFixedTestContainer() *sszquerypb.FixedTestContainer {
	fieldBytes32 := make([]byte, 32)
	for i := range fieldBytes32 {
		fieldBytes32[i] = byte(i + 24)
	}

	nestedValue2 := make([]byte, 32)
	for i := range nestedValue2 {
		nestedValue2[i] = byte(i + 56)
	}

	bitvector64 := bitfield.NewBitvector64()
	for i := range bitvector64 {
		bitvector64[i] = 0x42
	}

	bitvector512 := bitfield.NewBitvector512()
	for i := range bitvector512 {
		bitvector512[i] = 0x24
	}

	trailingField := make([]byte, 56)
	for i := range trailingField {
		trailingField[i] = byte(i + 88)
	}

	return &sszquerypb.FixedTestContainer{
		// Basic types
		FieldUint32: math.MaxUint32,
		FieldUint64: math.MaxUint64,
		FieldBool:   true,

		// Fixed-size bytes
		FieldBytes32: fieldBytes32,

		// Nested container
		Nested: &sszquerypb.FixedNestedContainer{
			Value1: 123,
			Value2: nestedValue2,
		},

		// Vector field
		VectorField: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24},

		// 2D bytes field
		TwoDimensionBytesField: [][]byte{
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
		},

		// Bitvector fields
		Bitvector64Field:  bitvector64,
		Bitvector512Field: bitvector512,

		// Trailing field
		TrailingField: trailingField,
	}
}

func getFixedTestContainerSpec() testutil.TestSpec {
	testContainer := createFixedTestContainer()

	return testutil.TestSpec{
		Name:     "FixedTestContainer",
		Type:     &sszquerypb.FixedTestContainer{},
		Instance: testContainer,
		PathTests: []testutil.PathTest{
			// Basic types
			{
				Path:     ".field_uint32",
				Expected: testContainer.FieldUint32,
			},
			{
				Path:     ".field_uint64",
				Expected: testContainer.FieldUint64,
			},
			{
				Path:     ".field_bool",
				Expected: testContainer.FieldBool,
			},
			// Fixed-size bytes
			{
				Path:     ".field_bytes32",
				Expected: testContainer.FieldBytes32,
			},
			// Nested container
			{
				Path:     ".nested",
				Expected: testContainer.Nested,
			},
			{
				Path:     ".nested.value1",
				Expected: testContainer.Nested.Value1,
			},
			{
				Path:     ".nested.value2",
				Expected: testContainer.Nested.Value2,
			},
			// Vector field
			{
				Path:     ".vector_field",
				Expected: testContainer.VectorField,
			},
			{
				Path:     ".vector_field[0]",
				Expected: testContainer.VectorField[0],
			},
			{
				Path:     ".vector_field[10]",
				Expected: testContainer.VectorField[10],
			},
			// 2D bytes field
			{
				Path:     ".two_dimension_bytes_field",
				Expected: testContainer.TwoDimensionBytesField,
			},
			{
				Path:     ".two_dimension_bytes_field[0]",
				Expected: testContainer.TwoDimensionBytesField[0],
			},
			{
				Path:     ".two_dimension_bytes_field[1]",
				Expected: testContainer.TwoDimensionBytesField[1],
			},
			// Bitvector fields
			{
				Path:     ".bitvector64_field",
				Expected: testContainer.Bitvector64Field,
			},
			{
				Path:     ".bitvector512_field",
				Expected: testContainer.Bitvector512Field,
			},
			// Trailing field
			{
				Path:     ".trailing_field",
				Expected: testContainer.TrailingField,
			},
		},
	}
}

func createVariableTestContainer() *sszquerypb.VariableTestContainer {
	leadingField := make([]byte, 32)
	for i := range leadingField {
		leadingField[i] = byte(i + 100)
	}

	trailingField := make([]byte, 56)
	for i := range trailingField {
		trailingField[i] = byte(i + 150)
	}

	nestedContainers := make([]*sszquerypb.FixedNestedContainer, 3)
	for i := range nestedContainers {
		value2 := make([]byte, 32)
		for j := range value2 {
			value2[j] = byte(j + i*32)
		}
		nestedContainers[i] = &sszquerypb.FixedNestedContainer{
			Value1: uint64(1000 + i),
			Value2: value2,
		}
	}

	bitlistField := bitfield.NewBitlist(256)
	bitlistField.SetBitAt(0, true)
	bitlistField.SetBitAt(10, true)
	bitlistField.SetBitAt(50, true)
	bitlistField.SetBitAt(100, true)
	bitlistField.SetBitAt(255, true)

	// Total size: 3 lists with lengths 32, 33, and 34 = 99 bytes
	nestedListField := make([][]byte, 3)
	for i := range nestedListField {
		nestedListField[i] = make([]byte, (32 + i)) // Different lengths for each sub-list
		for j := range nestedListField[i] {
			nestedListField[i][j] = byte(j + i*16)
		}
	}

	// Two VariableOuterContainer elements, each with two VariableInnerContainer elements
	variableContainerList := make([]*sszquerypb.VariableOuterContainer, 2)
	for i := range variableContainerList {
		// Inner1: 8 + 4 + 4 + (8*3) + (4*3) + 99 = 151 bytes
		inner1 := &sszquerypb.VariableNestedContainer{
			Value1:          42,
			FieldListUint64: []uint64{uint64(i), uint64(i + 1), uint64(i + 2)},
			NestedListField: nestedListField,
		}
		// Inner2: 8 + 4 + 4 + (8*2) + (4*3) + 99 = 143 bytes
		inner2 := &sszquerypb.VariableNestedContainer{
			Value1:          84,
			FieldListUint64: []uint64{uint64(i + 3), uint64(i + 4)},
			NestedListField: nestedListField,
		}
		// (4*2) + 151 + 143 = 302 bytes per VariableOuterContainer
		variableContainerList[i] = &sszquerypb.VariableOuterContainer{
			Inner_1: inner1,
			Inner_2: inner2,
		}
	}

	return &sszquerypb.VariableTestContainer{
		// Fixed leading field
		LeadingField: leadingField,

		// Variable-size lists
		FieldListUint64:    []uint64{100, 200, 300, 400, 500},
		FieldListContainer: nestedContainers,
		FieldListBytes32: [][]byte{
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
		},

		// Variable nested container
		Nested: &sszquerypb.VariableNestedContainer{
			Value1:          42,
			FieldListUint64: []uint64{1, 2, 3, 4, 5},
			NestedListField: nestedListField,
		},

		// Variable list of variable-sized containers
		VariableContainerList: variableContainerList,

		// Bitlist field
		BitlistField: bitlistField,

		// 2D bytes field
		NestedListField: nestedListField,

		// Fixed trailing field
		TrailingField: trailingField,
	}
}

func getVariableTestContainerSpec() testutil.TestSpec {
	testContainer := createVariableTestContainer()

	return testutil.TestSpec{
		Name:     "VariableTestContainer",
		Type:     &sszquerypb.VariableTestContainer{},
		Instance: testContainer,
		PathTests: []testutil.PathTest{
			// Fixed leading field
			{
				Path:     ".leading_field",
				Expected: testContainer.LeadingField,
			},
			// Variable-size list of uint64
			{
				Path:     ".field_list_uint64",
				Expected: testContainer.FieldListUint64,
			},
			{
				Path:     ".field_list_uint64[2]",
				Expected: testContainer.FieldListUint64[2],
			},
			// Variable-size list of (fixed-size) containers
			{
				Path:     ".field_list_container",
				Expected: testContainer.FieldListContainer,
			},
			// Accessing an element in the list of containers
			{
				Path:     ".field_list_container[0]",
				Expected: testContainer.FieldListContainer[0],
			},
			{
				Path:     ".field_list_container[1]",
				Expected: testContainer.FieldListContainer[1],
			},
			// Variable-size list of bytes32
			{
				Path:     ".field_list_bytes32",
				Expected: testContainer.FieldListBytes32,
			},
			// Variable nested container with every path
			{
				Path:     ".nested",
				Expected: testContainer.Nested,
			},
			{
				Path:     ".nested.value1",
				Expected: testContainer.Nested.Value1,
			},
			{
				Path:     ".nested.field_list_uint64",
				Expected: testContainer.Nested.FieldListUint64,
			},
			{
				Path:     ".nested.field_list_uint64[3]",
				Expected: testContainer.Nested.FieldListUint64[3],
			},
			{
				Path:     ".nested.nested_list_field",
				Expected: testContainer.Nested.NestedListField,
			},
			{
				Path:     ".nested.nested_list_field[0]",
				Expected: testContainer.Nested.NestedListField[0],
			},
			{
				Path:     ".nested.nested_list_field[1]",
				Expected: testContainer.Nested.NestedListField[1],
			},
			{
				Path:     ".nested.nested_list_field[2]",
				Expected: testContainer.Nested.NestedListField[2],
			},
			// Variable list of variable-sized containers
			{
				Path:     ".variable_container_list",
				Expected: testContainer.VariableContainerList,
			},
			{
				Path:     ".variable_container_list[0]",
				Expected: testContainer.VariableContainerList[0],
			},
			{
				Path:     ".variable_container_list[0].inner_1.field_list_uint64[1]",
				Expected: testContainer.VariableContainerList[0].Inner_1.FieldListUint64[1],
			},
			{
				Path:     ".variable_container_list[0].inner_2.field_list_uint64[1]",
				Expected: testContainer.VariableContainerList[0].Inner_2.FieldListUint64[1],
			},
			{
				Path:     ".variable_container_list[1]",
				Expected: testContainer.VariableContainerList[1],
			},
			{
				Path:     ".variable_container_list[1].inner_1.field_list_uint64[1]",
				Expected: testContainer.VariableContainerList[1].Inner_1.FieldListUint64[1],
			},
			{
				Path:     ".variable_container_list[1].inner_2.field_list_uint64[1]",
				Expected: testContainer.VariableContainerList[1].Inner_2.FieldListUint64[1],
			},
			// Bitlist field
			{
				Path:     ".bitlist_field",
				Expected: testContainer.BitlistField,
			},
			// 2D bytes field
			{
				Path:     ".nested_list_field",
				Expected: testContainer.NestedListField,
			},
			{
				Path:     ".nested_list_field[0]",
				Expected: testContainer.NestedListField[0],
			},
			{
				Path:     ".nested_list_field[1]",
				Expected: testContainer.NestedListField[1],
			},
			{
				Path:     ".nested_list_field[2]",
				Expected: testContainer.NestedListField[2],
			},
			// Fixed trailing field
			{
				Path:     ".trailing_field",
				Expected: testContainer.TrailingField,
			},
		},
	}
}
