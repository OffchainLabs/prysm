package blockchain

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
)

// recordNewPayloadRequestRoot derives the EIP-8025 new payload request root of
// a revealed execution payload and records it for the beacon block, marking the
// payload as available to prove.
//
// A failure here never fails payload import, it only leaves this node unable to
// judge execution proofs forthe block.
func (s *Service) recordNewPayloadRequestRoot(
	envelope interfaces.ROExecutionPayloadEnvelope,
	blockState state.BeaconState,
) {
	if !features.Get().EnableExecutionProofs || s.cfg.ExecutionProofCache == nil {
		return
	}

	root := envelope.BeaconBlockRoot()
	newPayloadRequestRoot, err := newPayloadRequestRoot(envelope, blockState)
	if err != nil {
		log.
			WithError(err).WithField("blockRoot", fmt.Sprintf("%#x", root)).
			Warning("Could not derive execution proof identifier for payload")
		return
	}

	s.cfg.ExecutionProofCache.SetNewPayloadRequestRoot(root, newPayloadRequestRoot)
}

// newPayloadRequestRoot builds the new payload request for a revealed payload
// and returns its hash tree root.
func newPayloadRequestRoot(
	envelope interfaces.ROExecutionPayloadEnvelope,
	state state.BeaconState,
) ([32]byte, error) {
	bid, err := state.LatestExecutionPayloadBid()
	if err != nil {
		return [32]byte{}, fmt.Errorf("latest execution payload bid: %w", err)
	}
	commitments := [][]byte{}
	if bid != nil {
		commitments = bid.BlobKzgCommitments()
	}

	newPayloadRequest, err := blocks.NewPayloadRequest(envelope, commitments)
	if err != nil {
		return [32]byte{}, fmt.Errorf("new payload request: %w", err)
	}

	root, err := newPayloadRequest.HashTreeRoot()
	if err != nil {
		return [32]byte{}, fmt.Errorf("hash tree root: %w", err)
	}

	return root, nil
}
