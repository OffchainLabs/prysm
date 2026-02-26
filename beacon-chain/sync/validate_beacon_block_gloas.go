package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

// validateExecutionPayloadBid validates execution payload bid gossip rules.
// [REJECT] The bid's parent (defined by bid.parent_block_root) equals the block's parent (defined by block.parent_root).
// [REJECT] The length of KZG commitments is less than or equal to the limitation defined in the consensus layer --
// i.e. validate that len(bid.blob_kzg_commitments) <= get_blob_parameters(get_current_epoch(state)).max_blobs_per_block
func (s *Service) validateExecutionPayloadBid(ctx context.Context, blk interfaces.ReadOnlyBeaconBlock) (pubsub.ValidationResult, error) {
	if blk.Version() < version.Gloas {
		return pubsub.ValidationAccept, nil
	}
	signedBid, err := blk.Body().SignedExecutionPayloadBid()
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "unable to read bid from block")
	}
	wrappedBid, err := consensusblocks.WrappedROSignedExecutionPayloadBid(signedBid)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "unable to wrap signed execution payload bid")
	}
	bid, err := wrappedBid.Bid()
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "unable to read bid from signed execution payload bid")
	}

	if bid.ParentBlockRoot() != blk.ParentRoot() {
		return pubsub.ValidationReject, errors.New("bid parent block root does not match block parent root")
	}

	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlockAtEpoch(slots.ToEpoch(blk.Slot()))
	if bid.BlobKzgCommitmentCount() > uint64(maxBlobsPerBlock) {
		return pubsub.ValidationReject, errors.Wrapf(errRejectCommitmentLen, "%d > %d", bid.BlobKzgCommitmentCount(), maxBlobsPerBlock)
	}
	return pubsub.ValidationAccept, nil
}

// validateExecutionPayloadBidParentSeen validates parent payload gossip rules.
// [IGNORE] The block's parent execution payload (defined by bid.parent_block_hash) has been seen
// (via gossip or non-gossip sources) (a client MAY queue blocks for processing once the parent payload is retrieved).
func (s *Service) validateExecutionPayloadBidParentSeen(_ context.Context, blk interfaces.ReadOnlyBeaconBlock) (pubsub.ValidationResult, error) {
	if blk.Version() < version.Gloas {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.chain.ParentPayloadReady(blk) {
		return pubsub.ValidationAccept, nil
	}
	return pubsub.ValidationIgnore, errors.New("parent payload not yet available")
}

// validateExecutionPayloadBidParentValid validates parent payload verification status.
// If execution_payload verification of block's execution payload parent by an execution node is complete:
// [REJECT] The block's execution payload parent (defined by bid.parent_block_hash) passes all validation.
func (s *Service) validateExecutionPayloadBidParentValid(_ context.Context, blk interfaces.ReadOnlyBeaconBlock) (pubsub.ValidationResult, error) {
	if blk.Version() < version.Gloas {
		return pubsub.ValidationAccept, nil
	}
	if s.hasBadPayload(blk.ParentRoot()) {
		return pubsub.ValidationReject, errors.New("parent payload is invalid")
	}
	return pubsub.ValidationAccept, nil
}
