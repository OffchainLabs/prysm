package state_native

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// SetExecutionPayloadBid sets the latest execution payload bid in the state.
func (b *BeaconState) SetExecutionPayloadBid(h interfaces.ROExecutionPayloadBid) error {
	if b.version < version.Gloas {
		return errNotSupported("SetExecutionPayloadBid", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	parentBlockHash := h.ParentBlockHash()
	parentBlockRoot := h.ParentBlockRoot()
	blockHash := h.BlockHash()
	randao := h.PrevRandao()
	blobKzgCommitmentsRoot := h.BlobKzgCommitmentsRoot()
	feeRecipient := h.FeeRecipient()
	b.latestExecutionPayloadBid = &ethpb.ExecutionPayloadBid{
		ParentBlockHash:        parentBlockHash[:],
		ParentBlockRoot:        parentBlockRoot[:],
		BlockHash:              blockHash[:],
		PrevRandao:             randao[:],
		GasLimit:               h.GasLimit(),
		BuilderIndex:           h.BuilderIndex(),
		Slot:                   h.Slot(),
		Value:                  h.Value(),
		ExecutionPayment:       h.ExecutionPayment(),
		BlobKzgCommitmentsRoot: blobKzgCommitmentsRoot[:],
		FeeRecipient:           feeRecipient[:],
	}
	b.markFieldAsDirty(types.LatestExecutionPayloadBid)

	return nil
}

// SetBuilderPendingPayment sets a builder pending payment at the specified index.
func (b *BeaconState) SetBuilderPendingPayment(index primitives.Slot, payment *ethpb.BuilderPendingPayment) error {
	if b.version < version.Gloas {
		return errNotSupported("SetBuilderPendingPayment", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.builderPendingPayments[index] = ethpb.CopyBuilderPendingPayment(payment)

	b.markFieldAsDirty(types.BuilderPendingPayments)
	return nil
}
