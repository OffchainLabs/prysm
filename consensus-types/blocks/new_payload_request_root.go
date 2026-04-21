package blocks

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	engine "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

// ComputeNewPayloadRequestRoot derives the hash tree root of the
// NewPayloadRequest associated with the given signed beacon block, using the
// same assembly as the gossip path. The block must carry a full execution
// payload (not a blinded header); callers are expected to reconstruct blinded
// blocks before calling.
func ComputeNewPayloadRequestRoot(signed interfaces.ReadOnlySignedBeaconBlock) ([32]byte, error) {
	if err := BeaconBlockIsNil(signed); err != nil {
		return [32]byte{}, fmt.Errorf("beacon block is nil: %w", err)
	}

	block := signed.Block()
	body := block.Body()

	execution, err := body.Execution()
	if err != nil {
		return [32]byte{}, fmt.Errorf("execution: %w", err)
	}

	transactions, err := execution.Transactions()
	if err != nil {
		return [32]byte{}, fmt.Errorf("transactions: %w", err)
	}

	withdrawals, err := execution.Withdrawals()
	if err != nil {
		return [32]byte{}, fmt.Errorf("withdrawals: %w", err)
	}

	blobGasUsed, err := execution.BlobGasUsed()
	if err != nil {
		return [32]byte{}, fmt.Errorf("blob gas used: %w", err)
	}

	excessBlobGas, err := execution.ExcessBlobGas()
	if err != nil {
		return [32]byte{}, fmt.Errorf("excess blob gas: %w", err)
	}

	executionPayload := &engine.ExecutionPayloadDeneb{
		ParentHash:    execution.ParentHash(),
		FeeRecipient:  execution.FeeRecipient(),
		StateRoot:     execution.StateRoot(),
		ReceiptsRoot:  execution.ReceiptsRoot(),
		LogsBloom:     execution.LogsBloom(),
		PrevRandao:    execution.PrevRandao(),
		BlockNumber:   execution.BlockNumber(),
		GasLimit:      execution.GasLimit(),
		GasUsed:       execution.GasUsed(),
		Timestamp:     execution.Timestamp(),
		ExtraData:     execution.ExtraData(),
		BaseFeePerGas: execution.BaseFeePerGas(),
		BlockHash:     execution.BlockHash(),
		Transactions:  transactions,
		Withdrawals:   withdrawals,
		BlobGasUsed:   blobGasUsed,
		ExcessBlobGas: excessBlobGas,
	}

	kzgCommitments, err := body.BlobKzgCommitments()
	if err != nil {
		return [32]byte{}, fmt.Errorf("blob kzg commitments: %w", err)
	}

	versionedHashes := make([][]byte, 0, len(kzgCommitments))
	for _, kzgCommitment := range kzgCommitments {
		versionedHash := primitives.ConvertKzgCommitmentToVersionedHash(kzgCommitment)
		versionedHashes = append(versionedHashes, versionedHash[:])
	}

	parentBlockRoot := block.ParentRoot()

	executionRequests, err := body.ExecutionRequests()
	if err != nil {
		return [32]byte{}, fmt.Errorf("execution requests: %w", err)
	}

	newPayloadRequest := engine.NewPayloadRequest{
		ExecutionPayload:  executionPayload,
		VersionedHashes:   versionedHashes,
		ParentBlockRoot:   parentBlockRoot[:],
		ExecutionRequests: executionRequests,
	}

	return newPayloadRequest.HashTreeRoot()
}
