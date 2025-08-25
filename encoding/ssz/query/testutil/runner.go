package testutil

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	ssz "github.com/prysmaticlabs/fastssz"
)

func RunStructTest(t *testing.T, spec TestSpec) {
	t.Run(spec.Name, func(t *testing.T) {
		info, err := query.AnalyzeObject(spec.Type)
		assert.NoError(t, err)

		testInstance := spec.Instance
		marshaller, ok := testInstance.(ssz.Marshaler)
		assert.Equal(t, true, ok, "Test instance must implement ssz.Marshaler, got %T", testInstance)

		marshalledData, err := marshaller.MarshalSSZ()
		assert.NoError(t, err)

		for _, pathTest := range spec.PathTests {
			t.Run(pathTest.Path, func(t *testing.T) {
				path, err := query.ParsePath(pathTest.Path)
				assert.NoError(t, err)

				_, offset, length, err := query.CalculateOffsetAndLength(info, path)
				assert.NoError(t, err)

				expectedRawBytes := marshalledData[offset : offset+length]
				rawBytes, err := marshalAny(pathTest.Expected)
				assert.NoError(t, err, "Marshalling expected value should not return an error")
				assert.DeepEqual(t, expectedRawBytes, rawBytes, "Extracted value should match expected")
			})
		}
	})
}
