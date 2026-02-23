package blockchain

import (
	consensus_blocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// getLookupParentRoot returns the root that serves as key to generate the parent state for the passed beacon block.
// if it is based on empty or it is pre-Gloas, it is the parent root of the block, otherwise if it is based on full it is
// the parent hash.
// The caller of this function should not hold a lock on forkchoice.
func (s *Service) getLookupParentRoot(b consensus_blocks.ROBlock) ([32]byte, error) {
	bl := b.Block()
	parentRoot := bl.ParentRoot()
	if b.Version() < version.Gloas {
		return parentRoot, nil
	}
	bidHash, err := s.cfg.ForkChoiceStore.BlockHash(parentRoot)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "failed to get block hash for parent root")
	}
	bid, err := bl.Body().SignedExecutionPayloadBid()
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "failed to get signed execution payload bid from block body")
	}
	if bid == nil || bid.Message == nil || len(bid.Message.ParentBlockHash) != 32 {
		return [32]byte{}, errors.New("invalid signed execution payload bid message")
	}
	parentHash := [32]byte(bid.Message.ParentBlockHash)
	if bidHash == parentHash {
		return parentHash, nil
	}
	return parentRoot, nil
}
