package blocks

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

// NewPayloadRequest builds the EIP-8025 new payload request for a revealed
// execution payload.
//
// NOTE: EIP-8025 beacon-chain.md specifies SSZNewPayloadRequest as a
// ProgressiveContainer. Prysm builds the bounded form instead, because the
// proof node derives this root the same way and is authoritative for it. See
// proto/engine/v1/eip8025.proto.
func NewPayloadRequest(
	envelope interfaces.ROExecutionPayloadEnvelope,
	blobKzgCommitments [][]byte,
) (*enginev1.SSZNewPayloadRequest, error) {
	if envelope == nil || envelope.IsNil() {
		return nil, errNilExecutionPayloadEnvelope
	}

	execution, err := envelope.Execution()
	if err != nil {
		return nil, fmt.Errorf("execution: %w", err)
	}

	// The new payload request mirrors the Gloas payload field for field, so read
	// it from the concrete message rather than through the fork-agnostic
	// ExecutionData interface, which does not expose the Gloas slot number.
	payload, ok := execution.Proto().(*enginev1.ExecutionPayloadGloas)
	if !ok {
		return nil, fmt.Errorf("execution payload is %T, want a Gloas payload", execution.Proto())
	}

	versionedHashes := make([][]byte, 0, len(blobKzgCommitments))
	for _, commitment := range blobKzgCommitments {
		versionedHash := primitives.ConvertKzgCommitmentToVersionedHash(commitment)
		versionedHashes = append(versionedHashes, versionedHash[:])
	}

	parentBeaconBlockRoot := envelope.ParentBeaconBlockRoot()

	return &enginev1.SSZNewPayloadRequest{
		ExecutionPayload:      statelessExecutionPayload(payload),
		VersionedHashes:       versionedHashes,
		ParentBeaconBlockRoot: parentBeaconBlockRoot[:],
		ExecutionRequests:     statelessExecutionRequests(envelope.ExecutionRequests()),
	}, nil
}

// statelessExecutionPayload reshapes a Gloas execution payload into its
// bounded stateless mirror. The two carry identical data and serialize
// identically; only their merkleization differs.
func statelessExecutionPayload(p *enginev1.ExecutionPayloadGloas) *enginev1.StatelessExecutionPayload {
	return &enginev1.StatelessExecutionPayload{
		ParentHash:      p.ParentHash,
		FeeRecipient:    p.FeeRecipient,
		StateRoot:       p.StateRoot,
		ReceiptsRoot:    p.ReceiptsRoot,
		LogsBloom:       p.LogsBloom,
		PrevRandao:      p.PrevRandao,
		BlockNumber:     p.BlockNumber,
		GasLimit:        p.GasLimit,
		GasUsed:         p.GasUsed,
		Timestamp:       p.Timestamp,
		ExtraData:       p.ExtraData,
		BaseFeePerGas:   p.BaseFeePerGas,
		BlockHash:       p.BlockHash,
		Transactions:    p.Transactions,
		Withdrawals:     p.Withdrawals,
		BlobGasUsed:     p.BlobGasUsed,
		ExcessBlobGas:   p.ExcessBlobGas,
		BlockAccessList: p.BlockAccessList,
		SlotNumber:      uint64(p.SlotNumber),
	}
}

// statelessExecutionRequests reshapes Gloas execution requests into their
// bounded stateless mirror.
func statelessExecutionRequests(r *enginev1.ExecutionRequestsGloas) *enginev1.StatelessExecutionRequests {
	if r == nil {
		return &enginev1.StatelessExecutionRequests{}
	}
	return &enginev1.StatelessExecutionRequests{
		Deposits:        r.Deposits,
		Withdrawals:     r.Withdrawals,
		Consolidations:  r.Consolidations,
		BuilderDeposits: r.BuilderDeposits,
		BuilderExits:    r.BuilderExits,
	}
}
