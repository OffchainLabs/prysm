package gloas

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// LookupParentRoot returns the state lookup key for a block pre-state.
// For pre-Gloas blocks (or when the parent is pre-Gloas), this is the block parent root.
// For Gloas blocks:
// - builds on empty => parent root
// - builds on full  => bid.parent_block_hash
func LookupParentRoot(
	block interfaces.ReadOnlyBeaconBlock,
	parentSlot primitives.Slot,
	parentNodeBlockHash [32]byte,
) ([32]byte, error) {
	parentRoot := block.ParentRoot()
	if slots.ToEpoch(parentSlot) < params.BeaconConfig().GloasForkEpoch || block.Version() < version.Gloas {
		return parentRoot, nil
	}

	signedBid, err := block.Body().SignedExecutionPayloadBid()
	if err != nil {
		return [32]byte{}, fmt.Errorf("failed to get signed execution payload bid from block body: %w", err)
	}
	if signedBid == nil || signedBid.Message == nil || len(signedBid.Message.ParentBlockHash) != 32 {
		return [32]byte{}, fmt.Errorf("invalid signed execution payload bid message")
	}

	parentBidHash := [32]byte(signedBid.Message.ParentBlockHash)
	if parentBidHash == parentNodeBlockHash {
		return parentBidHash, nil
	}
	return parentRoot, nil
}
