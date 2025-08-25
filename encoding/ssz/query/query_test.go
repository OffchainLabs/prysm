package query_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query/testutil"
	ssz_query "github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
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
			name:           "field_uint8",
			path:           ".field_uint8",
			expectedOffset: 0,
			expectedLength: 4,
		},
		{
			name:           "field_uint16",
			path:           ".field_uint16",
			expectedOffset: 4,
			expectedLength: 4,
		},
		{
			name:           "field_uint32",
			path:           ".field_uint32",
			expectedOffset: 8,
			expectedLength: 4,
		},
		{
			name:           "field_uint64",
			path:           ".field_uint64",
			expectedOffset: 12,
			expectedLength: 8,
		},
		// Boolean type
		{
			name:           "field_bool",
			path:           ".field_bool",
			expectedOffset: 20,
			expectedLength: 1,
		},
		// Fixed-size bytes
		{
			name:           "field_bytes8",
			path:           ".field_bytes8",
			expectedOffset: 21,
			expectedLength: 8,
		},
		{
			name:           "field_bytes16",
			path:           ".field_bytes16",
			expectedOffset: 29,
			expectedLength: 16,
		},
		{
			name:           "field_bytes32",
			path:           ".field_bytes32",
			expectedOffset: 45,
			expectedLength: 32,
		},
		// Nested container
		{
			name:           "nested container",
			path:           ".nested",
			expectedOffset: 77,
			expectedLength: 40,
		},
		{
			name:           "nested value1",
			path:           ".nested.value1",
			expectedOffset: 77,
			expectedLength: 8,
		},
		{
			name:           "nested value2",
			path:           ".nested.value2",
			expectedOffset: 85,
			expectedLength: 32,
		},
		// Vector field
		{
			name:           "vector field",
			path:           ".vector_field",
			expectedOffset: 117,
			expectedLength: 192, // 24 * 8 bytes
		},
		// Trailing field
		{
			name:           "field_trailing",
			path:           ".field_trailing",
			expectedOffset: 309,
			expectedLength: 56,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := query.ParsePath(tt.path)
			assert.NoError(t, err)

			info, err := query.AnalyzeObject(&ssz_query.FixedTestContainer{})
			assert.NoError(t, err)

			_, offset, length, err := query.CalculateOffsetAndLength(info, path)
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedOffset, offset, "Expected offset to be %d", tt.expectedOffset)
			assert.Equal(t, tt.expectedLength, length, "Expected length to be %d", tt.expectedLength)
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
	fieldBytes8 := make([]byte, 8)
	for i := range fieldBytes8 {
		fieldBytes8[i] = byte(i)
	}
	
	fieldBytes16 := make([]byte, 16)
	for i := range fieldBytes16 {
		fieldBytes16[i] = byte(i + 8)
	}
	
	fieldBytes32 := make([]byte, 32)
	for i := range fieldBytes32 {
		fieldBytes32[i] = byte(i + 24)
	}
	
	nestedValue2 := make([]byte, 32)
	for i := range nestedValue2 {
		nestedValue2[i] = byte(i + 56)
	}
	
	fieldTrailing := make([]byte, 56)
	for i := range fieldTrailing 	{
		fieldTrailing[i] = byte(i + 88)
	}

	return &ssz_query.FixedTestContainer{
		// Basic types
		FieldUint8:    255,    // Max value for uint8 representation
		FieldUint16:   65535,  // Max value for uint16 representation  
		FieldUint32:   4294967295, // Max value for uint32
		FieldUint64:   18446744073709551615, // Max value for uint64
		FieldBool:     true,
		
		// Fixed-size bytes
		FieldBytes8:   fieldBytes8,
		FieldBytes16:  fieldBytes16,
		FieldBytes32:  fieldBytes32,
		
		// Nested container
		Nested: &ssz_query.FixedNestedContainer{
			Value1: 123,
			Value2: nestedValue2,
		},
		
		// Vector field
		VectorField: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24},
		
		// Trailing field
		FieldTrailing: fieldTrailing,
	}
}

func getFixedTestContainerSpec() testutil.TestSpec {
	testContainer := createFixedTestContainer().(*ssz_query.FixedTestContainer)

	return testutil.TestSpec{
		Name:     "FixedTestContainer",
		Type:     ssz_query.FixedTestContainer{},
		Instance: testContainer,
		PathTests: []testutil.PathTest{
			// Basic types
			{
				Path:     ".field_uint8",
				Expected: testContainer.FieldUint8,
			},
			{
				Path:     ".field_uint16",
				Expected: testContainer.FieldUint16,
			},
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
				Path:     ".field_bytes8",
				Expected: testContainer.FieldBytes8,
			},
			{
				Path:     ".field_bytes16",
				Expected: testContainer.FieldBytes16,
			},
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
				Path:     ".field_trailing",
				Expected: testContainer.FieldTrailing,
			},
		},
	}
}