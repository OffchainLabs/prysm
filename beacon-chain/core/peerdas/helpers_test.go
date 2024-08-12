package peerdas_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/sirupsen/logrus"
)

func deterministicRandomness(seed int64) [32]byte {
	// Converts an int64 to a byte slice
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, seed)
	if err != nil {
		logrus.WithError(err).Error("Failed to write int64 to bytes buffer")
		return [32]byte{}
	}
	bytes := buf.Bytes()

	return sha256.Sum256(bytes)
}

// Returns a serialized random field element in big-endian
func GetRandFieldElement(seed int64) [32]byte {
	bytes := deterministicRandomness(seed)
	var r fr.Element
	r.SetBytes(bytes[:])

	return GoKZG.SerializeScalar(r)
}

// Returns a random blob using the passed seed as entropy
func GetRandBlob(seed int64) kzg.Blob {
	var blob kzg.Blob
	bytesPerBlob := GoKZG.ScalarsPerBlob * GoKZG.SerializedScalarSize
	for i := 0; i < bytesPerBlob; i += GoKZG.SerializedScalarSize {
		fieldElementBytes := GetRandFieldElement(seed + int64(i))
		copy(blob[i:i+GoKZG.SerializedScalarSize], fieldElementBytes[:])
	}
	return blob
}

func GenerateCommitmentAndProof(blob *kzg.Blob) (*kzg.Commitment, *kzg.Proof, error) {
	commitment, err := kzg.BlobToKZGCommitment(blob)
	if err != nil {
		return nil, nil, err
	}
	proof, err := kzg.ComputeBlobKZGProof(blob, commitment)
	if err != nil {
		return nil, nil, err
	}
	return &commitment, &proof, err
}

func TestVerifyDataColumnSidecarKZGProofs(t *testing.T) {
	dbBlock := util.NewBeaconBlockDeneb()
	require.NoError(t, kzg.Start())

	var (
		comms [][]byte
		blobs []kzg.Blob
	)
	for i := int64(0); i < 6; i++ {
		blob := GetRandBlob(i)
		commitment, _, err := GenerateCommitmentAndProof(&blob)
		require.NoError(t, err)
		comms = append(comms, commitment[:])
		blobs = append(blobs, blob)
	}

	dbBlock.Block.Body.BlobKzgCommitments = comms
	sBlock, err := blocks.NewSignedBeaconBlock(dbBlock)
	require.NoError(t, err)
	sCars, err := peerdas.DataColumnSidecars(sBlock, blobs)
	require.NoError(t, err)

	for i, sidecar := range sCars {
		roCol, err := blocks.NewRODataColumn(sidecar)
		require.NoError(t, err)
		verified, err := peerdas.VerifyDataColumnSidecarKZGProofs(roCol)
		require.NoError(t, err)
		require.Equal(t, true, verified, fmt.Sprintf("sidecar %d failed", i))
	}
}

func TestHypergeomCDF(t *testing.T) {
	// Test case from https://en.wikipedia.org/wiki/Hypergeometric_distribution
	// Population size: 1000, number of successes in population: 500, sample size: 10, number of successes in sample: 5
	// Expected result: 0.072
	const (
		expected = 0.0796665913283742
		margin   = 0.000001
	)

	actual := peerdas.HypergeomCDF(5, 128, 65, 16)
	require.Equal(t, true, expected-margin <= actual && actual <= expected+margin)
}

func TestExtendedSampleCount(t *testing.T) {
	const samplesPerSlot = 16

	testCases := []struct {
		name                string
		allowedMissings     uint64
		extendedSampleCount uint64
	}{
		{name: "allowedMissings=0", allowedMissings: 0, extendedSampleCount: 16},
		{name: "allowedMissings=1", allowedMissings: 1, extendedSampleCount: 20},
		{name: "allowedMissings=2", allowedMissings: 2, extendedSampleCount: 24},
		{name: "allowedMissings=3", allowedMissings: 3, extendedSampleCount: 27},
		{name: "allowedMissings=4", allowedMissings: 4, extendedSampleCount: 29},
		{name: "allowedMissings=5", allowedMissings: 5, extendedSampleCount: 32},
		{name: "allowedMissings=6", allowedMissings: 6, extendedSampleCount: 35},
		{name: "allowedMissings=7", allowedMissings: 7, extendedSampleCount: 37},
		{name: "allowedMissings=8", allowedMissings: 8, extendedSampleCount: 40},
		{name: "allowedMissings=9", allowedMissings: 9, extendedSampleCount: 42},
		{name: "allowedMissings=10", allowedMissings: 10, extendedSampleCount: 44},
		{name: "allowedMissings=11", allowedMissings: 11, extendedSampleCount: 47},
		{name: "allowedMissings=12", allowedMissings: 12, extendedSampleCount: 49},
		{name: "allowedMissings=13", allowedMissings: 13, extendedSampleCount: 51},
		{name: "allowedMissings=14", allowedMissings: 14, extendedSampleCount: 53},
		{name: "allowedMissings=15", allowedMissings: 15, extendedSampleCount: 55},
		{name: "allowedMissings=16", allowedMissings: 16, extendedSampleCount: 57},
		{name: "allowedMissings=17", allowedMissings: 17, extendedSampleCount: 59},
		{name: "allowedMissings=18", allowedMissings: 18, extendedSampleCount: 61},
		{name: "allowedMissings=19", allowedMissings: 19, extendedSampleCount: 63},
		{name: "allowedMissings=20", allowedMissings: 20, extendedSampleCount: 65},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := peerdas.ExtendedSampleCount(samplesPerSlot, tc.allowedMissings)
			require.Equal(t, tc.extendedSampleCount, result)
		})
	}
}
