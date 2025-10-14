package testutil

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	ssz "github.com/prysmaticlabs/fastssz"
)

func RunStructTest(t *testing.T, spec TestSpec) {
	t.Run(spec.Name, func(t *testing.T) {
		object, ok := spec.Type.(query.SSZObject)
		require.Equal(t, true, ok, "spec.Type must implement SSZObject interface")
		require.NotNil(t, object, "spec.Type must not be nil")
		info, err := query.AnalyzeObject(object)
		require.NoError(t, err)

		testInstance := spec.Instance
		err = query.PopulateVariableLengthInfo(info, testInstance)
		require.NoError(t, err)

		marshaller, ok := testInstance.(ssz.Marshaler)
		require.Equal(t, true, ok, "Test instance must implement ssz.Marshaler, got %T", testInstance)

		marshalledData, err := marshaller.MarshalSSZ()
		require.NoError(t, err)

		for _, pathTest := range spec.PathTests {
			t.Run(pathTest.Path, func(t *testing.T) {
				path, err := query.ParsePath(pathTest.Path)
				require.NoError(t, err)

				_, offset, length, err := query.CalculateOffsetAndLength(info, path)
				require.NoError(t, err)

				actualRawBytes := marshalledData[offset : offset+length]
				expectedRawBytes, err := marshalAny(pathTest.Expected)
				require.NoError(t, err, "Marshalling expected value should not return an error")
				require.DeepEqual(t, actualRawBytes, expectedRawBytes, "Extracted value should match expected")
			})
		}
	})
}
