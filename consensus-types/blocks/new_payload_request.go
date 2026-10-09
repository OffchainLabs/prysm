package blocks

import (
	"fmt"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

// NewPayloadRequestRoot returns the hash tree root of the Gloas new payload request of a revealed execution payload,
// which identifies the payload in its EIP-8025 execution proofs. It MUST match the root zkboost proves.
//
// WARNING: the root DELIBERATELY DRIFTS from the specification, where `NewPayloadRequest` is a
// `ProgressiveContainer`. It is the root of zkboost v0.10.0 and later, computed by the reference stateless guest
// library they pin (github.com/eth-act/ere-guests v0.17.0, crates/stateless-validator-common): a PLAIN container of
// the progressive execution payload, the progressive list of versioned hashes, the parent beacon block root and the
// progressive execution requests.
func NewPayloadRequestRoot(
	envelope interfaces.ROExecutionPayloadEnvelope,
	blobKzgCommitments [][]byte,
) ([fieldparams.RootLength]byte, error) {
	if envelope == nil || envelope.IsNil() {
		return [fieldparams.RootLength]byte{}, errNilExecutionPayloadEnvelope
	}

	execution, err := envelope.Execution()
	if err != nil {
		return [fieldparams.RootLength]byte{}, fmt.Errorf("execution: %w", err)
	}

	payload, ok := execution.Proto().(*enginev1.ExecutionPayloadGloas)
	if !ok {
		return [fieldparams.RootLength]byte{}, fmt.Errorf("execution payload is %T, want a Gloas payload", execution.Proto())
	}

	versionedHashes := make([][]byte, 0, len(blobKzgCommitments))
	for _, commitment := range blobKzgCommitments {
		versionedHash := primitives.ConvertKzgCommitmentToVersionedHash(commitment)
		versionedHashes = append(versionedHashes, versionedHash[:])
	}

	parentBeaconBlockRoot := envelope.ParentBeaconBlockRoot()

	return newPayloadRequestRoot(payload, versionedHashes, parentBeaconBlockRoot[:], envelope.ExecutionRequests())
}

// newPayloadRequestRoot merkleizes the Gloas new payload request as zkboost does, see NewPayloadRequestRoot.
func newPayloadRequestRoot(
	payload *enginev1.ExecutionPayloadGloas,
	versionedHashes [][]byte,
	parentBeaconBlockRoot []byte,
	requests *enginev1.ExecutionRequestsGloas,
) ([fieldparams.RootLength]byte, error) {
	if requests == nil {
		requests = &enginev1.ExecutionRequestsGloas{}
	}

	if len(parentBeaconBlockRoot) != fieldparams.RootLength {
		return [fieldparams.RootLength]byte{}, fmt.Errorf("parent beacon block root has length %d", len(parentBeaconBlockRoot))
	}

	hh := ssz.DefaultHasherPool.Get()
	defer ssz.DefaultHasherPool.Put(hh)

	indx := hh.Index()

	// Field 0: execution_payload
	if err := payload.HashTreeRootWith(hh); err != nil {
		return [fieldparams.RootLength]byte{}, fmt.Errorf("execution payload: %w", err)
	}

	// Field 1: versioned_hashes, a progressive list of Bytes32
	{
		subIndx := hh.Index()
		for i, versionedHash := range versionedHashes {
			if len(versionedHash) != fieldparams.RootLength {
				return [fieldparams.RootLength]byte{}, fmt.Errorf("versioned hash %d has length %d", i, len(versionedHash))
			}
			hh.AppendBytes32(versionedHash)
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(versionedHashes)))
	}

	// Field 2: parent_beacon_block_root
	hh.PutBytes(parentBeaconBlockRoot)

	// Field 3: execution_requests
	if err := requests.HashTreeRootWith(hh); err != nil {
		return [fieldparams.RootLength]byte{}, fmt.Errorf("execution requests: %w", err)
	}

	hh.Merkleize(indx)

	return hh.HashRoot()
}
