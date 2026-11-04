package blocks

import (
	"encoding/binary"
	"os"
	"testing"

	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// TestNewPayloadRequestRoot pins the new payload request root to zkboost's. The root is the execution proof
// identifier, so any mismatch makes every execution proof unverifiable.
//
// WARNING: the expected root is NOT the one the EIP-8025 specification defines (see NewPayloadRequestRoot). It is the
// one of zkboost v0.10.0 and later, computed by the reference stateless guest library they pin:
// github.com/eth-act/ere-guests v0.17.0, crates/stateless-validator-common.
//
// testdata/zkboost_new_payload_request.ssz is the new payload request of the Gloas block 93354 of
// glamsterdam-devnet-8, taken from the stateless input fixture of github.com/eth-act/zkboost
// (crates/server/tests/fixture/stateless_input_amsterdam.ssz, commit 7f8bb2f). Its serialization does not depend on
// the merkleization.
func TestNewPayloadRequestRoot(t *testing.T) {
	const expectedRoot = "0x8c3a890206a189727e151767653f846ccddbd269eb29fb0a2f97371f23a481c6"

	data, err := os.ReadFile("testdata/zkboost_new_payload_request.ssz")
	require.NoError(t, err)

	// The request is a container of three variable-size fields, by offset, and the fixed-size parent beacon block
	// root.
	const fixedSize = 4 + 4 + 32 + 4
	require.Equal(t, true, len(data) >= fixedSize)
	payloadOffset := binary.LittleEndian.Uint32(data[0:4])
	versionedHashesOffset := binary.LittleEndian.Uint32(data[4:8])
	parentBeaconBlockRoot := data[8:40]
	requestsOffset := binary.LittleEndian.Uint32(data[40:44])
	require.Equal(t, uint32(fixedSize), payloadOffset)

	payload := &enginev1.ExecutionPayloadGloas{}
	require.NoError(t, payload.UnmarshalSSZ(data[payloadOffset:versionedHashesOffset]))

	encodedVersionedHashes := data[versionedHashesOffset:requestsOffset]
	require.Equal(t, 0, len(encodedVersionedHashes)%32)
	versionedHashes := make([][]byte, 0, len(encodedVersionedHashes)/32)
	for i := 0; i < len(encodedVersionedHashes); i += 32 {
		versionedHashes = append(versionedHashes, encodedVersionedHashes[i:i+32])
	}

	requests := &enginev1.ExecutionRequestsGloas{}
	require.NoError(t, requests.UnmarshalSSZ(data[requestsOffset:]))

	t.Run("fixture", func(t *testing.T) {
		root, err := newPayloadRequestRoot(payload, versionedHashes, parentBeaconBlockRoot, requests)
		require.NoError(t, err)
		require.Equal(t, expectedRoot, hexutil.Encode(root[:]))
	})

	t.Run("nil execution requests are empty", func(t *testing.T) {
		nilRoot, err := newPayloadRequestRoot(payload, versionedHashes, parentBeaconBlockRoot, nil)
		require.NoError(t, err)
		emptyRoot, err := newPayloadRequestRoot(payload, versionedHashes, parentBeaconBlockRoot, &enginev1.ExecutionRequestsGloas{})
		require.NoError(t, err)
		require.Equal(t, emptyRoot, nilRoot)
	})

	t.Run("bad versioned hash", func(t *testing.T) {
		_, err := newPayloadRequestRoot(payload, [][]byte{{0x01}}, parentBeaconBlockRoot, requests)
		require.ErrorContains(t, "versioned hash 0 has length 1", err)
	})

	t.Run("bad parent beacon block root", func(t *testing.T) {
		_, err := newPayloadRequestRoot(payload, versionedHashes, []byte{0x01}, requests)
		require.ErrorContains(t, "parent beacon block root has length 1", err)
	})

	t.Run("nil envelope", func(t *testing.T) {
		_, err := NewPayloadRequestRoot(nil, nil)
		require.ErrorIs(t, err, errNilExecutionPayloadEnvelope)
	})
}
