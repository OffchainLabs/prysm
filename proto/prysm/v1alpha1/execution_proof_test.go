package eth_test

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestExecutionProofEnvelope_HashTreeRoot pins the merkleization of proof_data to the bounded
// ByteList[MAX_PROOF_SIZE] of consensus-specs PR #5593, which zkboost signs. With the ProgressiveList[Byte] of
// consensus-specs master, Prysm rejects every zkboost signature.
func TestExecutionProofEnvelope_HashTreeRoot(t *testing.T) {
	hash := func(a, b [32]byte) [32]byte {
		return sha256.Sum256(append(a[:], b[:]...))
	}

	var proofDataChunk, proofTypeChunk, beaconBlockRoot [32]byte
	proofDataChunk[0], proofDataChunk[1] = 0xaa, 0xbb
	proofTypeChunk[0] = byte(eth.ProofTypeEthrexSP1)
	beaconBlockRoot[0] = 0x01

	// MAX_PROOF_SIZE = 4194304 bytes = 2^17 chunks: one chunk padded with 17 levels of zero hashes.
	proofDataRoot := proofDataChunk
	var zero [32]byte
	for range 17 {
		proofDataRoot = hash(proofDataRoot, zero)
		zero = hash(zero, zero)
	}
	var length [32]byte
	binary.LittleEndian.PutUint64(length[:], 2)
	proofDataRoot = hash(proofDataRoot, length)

	// Three fields, padded to four.
	want := hash(hash(proofDataRoot, proofTypeChunk), hash(beaconBlockRoot, [32]byte{}))

	envelope := &eth.ExecutionProofEnvelope{
		ProofData:       []byte{0xaa, 0xbb},
		ProofType:       []byte{byte(eth.ProofTypeEthrexSP1)},
		BeaconBlockRoot: beaconBlockRoot[:],
	}
	got, err := envelope.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, want, got)
}
