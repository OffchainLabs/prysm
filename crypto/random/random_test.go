package random

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
)

func TestDeterministicRandomness(t *testing.T) {
	seed := int64(123)
	r1 := DeterministicRandomness(seed)
	r2 := DeterministicRandomness(seed)
	assert.DeepEqual(t, r1, r2, "Same seed should produce same output")

	// Test different seeds produce different outputs
	r3 := DeterministicRandomness(seed + 1)
	assert.NotEqual(t, r1, r3, "Different seeds should produce different outputs")
}

func TestGetRandFieldElement(t *testing.T) {
	seed := int64(123)
	r1 := GetRandFieldElement(seed)
	r2 := GetRandFieldElement(seed)
	assert.DeepEqual(t, r1, r2, "Same seed should produce same output")

	// Test different seeds produce different outputs
	r3 := GetRandFieldElement(seed + 1)
	assert.NotEqual(t, r1, r3, "Different seeds should produce different outputs")
}

func TestGetRandBlob(t *testing.T) {
	seed := int64(123)
	r1 := GetRandBlob(seed)
	r2 := GetRandBlob(seed)
	assert.DeepEqual(t, r1, r2, "Same seed should produce same blob")

	expectedSize := GoKZG.ScalarsPerBlob * GoKZG.SerializedScalarSize
	assert.Equal(t, expectedSize, len(r1), "Blob should have correct size")

	r3 := GetRandBlob(seed + 1)
	assert.NotEqual(t, r1, r3, "Different seeds should produce different blobs")
}

func TestGetRandBlobElements(t *testing.T) {
	seed := int64(123)
	blob := GetRandBlob(seed)

	// Check that each field element in the blob matches what we'd get from GetRandFieldElement
	for i := 0; i < GoKZG.ScalarsPerBlob; i++ {
		start := i * GoKZG.SerializedScalarSize
		end := start + GoKZG.SerializedScalarSize

		blobElement := [32]byte{}
		copy(blobElement[:], blob[start:end])

		expectedElement := GetRandFieldElement(seed + int64(i*GoKZG.SerializedScalarSize))
		require.DeepEqual(t, expectedElement, blobElement, "Field element in blob doesn't match expected value")
	}
}
