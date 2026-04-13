package sync

import (
	"bytes"
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

// validateExecutionPayloadBid validates execution payload bid gossip rules.
// [REJECT] The bid's parent (defined by bid.parent_block_root) equals the block's parent (defined by block.parent_root).
// [REJECT] The length of KZG commitments is less than or equal to the limitation defined in the consensus layer.
// [REJECT] If parent was FULL, hash_tree_root(block.body.parent_execution_requests) == parent_bid.execution_requests_root.
// [REJECT] If parent was EMPTY, block.body.parent_execution_requests == ExecutionRequests().
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

// validateParentExecutionRequests checks the REJECT conditions for parent_execution_requests.
func (s *Service) validateParentExecutionRequests(ctx context.Context, blk interfaces.ReadOnlyBeaconBlock, bid *ethpb.SignedExecutionPayloadBid) (pubsub.ValidationResult, error) {
	parentRoot := blk.ParentRoot()
	parentBlock, err := s.cfg.beaconDB.Block(ctx, parentRoot)
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "could not get parent block")
	}
	// Pre-Gloas parent: parent_execution_requests must be empty.
	if parentBlock.Block().Version() < version.Gloas {
		parentExecutionRequests, err := blk.Body().ParentExecutionRequests()
		if err != nil {
			return pubsub.ValidationIgnore, errors.Wrap(err, "could not get parent execution requests")
		}
		if !isEmptyExecutionRequests(parentExecutionRequests) {
			return pubsub.ValidationReject, errors.New("pre-Gloas parent but parent_execution_requests is non-empty")
		}
		return pubsub.ValidationAccept, nil
	}
	parentSignedBid, err := parentBlock.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "could not get parent bid")
	}

	parentExecutionRequests, err := blk.Body().ParentExecutionRequests()
	if err != nil {
		return pubsub.ValidationIgnore, errors.Wrap(err, "could not get parent execution requests")
	}

	isParentFull := bytes.Equal(bid.Message.ParentBlockHash, parentSignedBid.Message.BlockHash)
	if isParentFull {
		root, err := parentExecutionRequests.HashTreeRoot()
		if err != nil {
			return pubsub.ValidationIgnore, errors.Wrap(err, "could not compute parent execution requests root")
		}
		if !bytes.Equal(root[:], parentSignedBid.Message.ExecutionRequestsRoot) {
			return pubsub.ValidationReject, errors.New("parent execution requests root does not match parent bid commitment")
		}
	} else {
		if !isEmptyExecutionRequests(parentExecutionRequests) {
			return pubsub.ValidationReject, errors.New("parent was empty but parent_execution_requests is non-empty")
		}
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

// requestPayloadEnvelope asks a random peer for the execution payload
// envelope identified by root and feeds any response through
// ReceiveExecutionPayloadEnvelope.
func (s *Service) requestPayloadEnvelope(root [32]byte) {
	bestPeers := s.getBestPeers()
	if len(bestPeers) == 0 {
		return
	}
	pid := bestPeers[rand.NewGenerator().Int()%len(bestPeers)]
	req := p2ptypes.ExecutionPayloadEnvelopesByRootReq{root}
	envelopes, err := SendExecutionPayloadEnvelopesByRootRequest(s.ctx, s.cfg.clock, s.cfg.p2p, pid, s.ctxMap, &req)
	if err != nil {
		log.WithError(err).Debug("Could not request payload envelope by root")
		return
	}
	if len(envelopes) == 0 {
		log.Debug("No payload envelopes returned by peer")
		return
	}
	if len(envelopes) > 1 {
		log.Warn("Multiple payload envelopes returned by peer, expected at most one")
	}
	for _, env := range envelopes {
		wrapped, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(env)
		if err != nil {
			log.WithError(err).Debug("Could not wrap requested payload envelope")
			continue
		}
		if err := s.cfg.chain.ReceiveExecutionPayloadEnvelope(s.ctx, wrapped); err != nil {
			if blockchain.IsInvalidBlock(err) {
				s.setBadPayload(s.ctx, root)
			}
			log.WithError(err).Debug("Could not process requested payload envelope")
		}
	}
}

func isEmptyExecutionRequests(r *enginev1.ExecutionRequests) bool {
	if r == nil {
		return true
	}
	return len(r.Deposits) == 0 && len(r.Withdrawals) == 0 && len(r.Consolidations) == 0
}
