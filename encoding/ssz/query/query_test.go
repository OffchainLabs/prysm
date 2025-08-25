package query_test

import (
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query/testutil"
	"github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	sszquerypb "github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestCalculateOffsetAndLength(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedOffset uint64
		expectedLength uint64
	}{
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
		// Trailing field
		{
			name:           "trailing_field",
			path:           ".trailing_field",
			expectedOffset: 277,
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
}

func TestRoundTripSszInfo(t *testing.T) {
	specs := []testutil.TestSpec{
		getFixedTestContainerSpec(),
	}

	for _, spec := range specs {
		testutil.RunStructTest(t, spec)
	}
}

func createFixedTestContainer() any {
	fieldBytes32 := make([]byte, 32)
	for i := range fieldBytes32 {
		fieldBytes32[i] = byte(i + 24)
	}

	nestedValue2 := make([]byte, 32)
	for i := range nestedValue2 {
		nestedValue2[i] = byte(i + 56)
	}

	trailingField := make([]byte, 56)
	for i := range trailingField {
		trailingField[i] = byte(i + 88)
	}

	return &ssz_query.FixedTestContainer{
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

		// Trailing field
		TrailingField: trailingField,
	}
}

func getFixedTestContainerSpec() testutil.TestSpec {
	testContainer := createFixedTestContainer().(*sszquerypb.FixedTestContainer)

	return testutil.TestSpec{
		Name:     "FixedTestContainer",
		Type:     sszquerypb.FixedTestContainer{},
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
			// Trailing field
			{
				Path:     ".trailing_field",
				Expected: testContainer.TrailingField,
			},
		},
	}
}
