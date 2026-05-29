package blocks

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	engine "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

// NewPayloadRequestRoot derives the hash tree root of the
// NewPayloadRequest associated with the given execution payload envelope.
func NewPayloadRequestRoot(envelope interfaces.ROExecutionPayloadEnvelope, blobKzgCommitments [][]byte) ([32]byte, error) {
	if envelope == nil || envelope.IsNil() {
		return [32]byte{}, fmt.Errorf("nil envelope")
	}

	execution, err := envelope.Execution()
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

	versionedHashes := make([][]byte, 0, len(blobKzgCommitments))
	for _, kzgCommitment := range blobKzgCommitments {
		versionedHash := primitives.ConvertKzgCommitmentToVersionedHash(kzgCommitment)
		versionedHashes = append(versionedHashes, versionedHash[:])
	}

	parentBlockRoot := envelope.ParentBeaconBlockRoot()
	newPayloadRequest := engine.NewPayloadRequest{
		ExecutionPayload:  executionPayload,
		VersionedHashes:   versionedHashes,
		ParentBlockRoot:   parentBlockRoot[:],
		ExecutionRequests: envelope.ExecutionRequests(),
	}

	return newPayloadRequest.HashTreeRoot()
}
