package validator

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	blockfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/block"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// setGloasExecutionData creates an execution payload bid from the local payload,
// sets it on the block, and caches the execution payload envelope for later
// retrieval by the validator client.
func (vs *Server) setGloasExecutionData(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
) error {
	_, span := trace.StartSpan(ctx, "ProposerServer.setGloasExecutionData")
	defer span.End()

	if local == nil || local.ExecutionData == nil {
		return errors.New("local execution payload is nil")
	}

	// Create execution payload bid from the local payload.
	parentRoot := sBlk.Block().ParentRoot()
	bid, err := vs.createExecutionPayloadBid(
		local.ExecutionData,
		primitives.BuilderIndex(sBlk.Block().ProposerIndex()),
		parentRoot[:],
		sBlk.Block().Slot(),
		local.BlobsBundler,
	)
	if err != nil {
		return errors.Wrap(err, "could not create execution payload bid")
	}

	// The bid signature is set to the BLS point-at-infinity as a placeholder.
	// For self-building (BUILDER_INDEX_SELF_BUILD), the VC must replace this
	// with a real signature using the proposer's key before broadcasting.
	signedBid := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: common.InfiniteSignature[:],
	}
	if err := sBlk.SetSignedExecutionPayloadBid(signedBid); err != nil {
		return errors.Wrap(err, "could not set signed execution payload bid")
	}

	// Cache the execution payload envelope for later retrieval by the VC.
	envelope := vs.createExecutionPayloadEnvelope(
		local.ExecutionData,
		local.ExecutionRequests,
		primitives.BuilderIndex(sBlk.Block().ProposerIndex()),
		sBlk.Block().Slot(),
		local.BlobsBundler,
	)
	vs.cacheExecutionPayloadEnvelope(envelope)

	return nil
}

// getPayloadAttestations returns payload attestations for inclusion in a GLOAS block.
// These attest to the payload timeliness from the previous slot's PTC.
func (vs *Server) getPayloadAttestations(ctx context.Context, head state.BeaconState, slot primitives.Slot) []*ethpb.PayloadAttestation {
	// TODO: Implement payload attestation retrieval from pool.
	// This requires:
	// 1. A PayloadAttestationPool to collect PTC votes
	// 2. Aggregation of individual PayloadAttestationMessages into PayloadAttestations
	// For now, return empty - blocks are valid without payload attestations.
	return []*ethpb.PayloadAttestation{}
}

// createExecutionPayloadBid creates an ExecutionPayloadBid from a full execution payload.
// For local block building, the beacon node acts as its own builder.
func (vs *Server) createExecutionPayloadBid(
	executionData interfaces.ExecutionData,
	builderIndex primitives.BuilderIndex,
	parentBlockRoot []byte,
	slot primitives.Slot,
	blobsBundler enginev1.BlobsBundler,
) (*ethpb.ExecutionPayloadBid, error) {
	if executionData == nil || executionData.IsNil() {
		return nil, errors.New("execution data is nil")
	}

	// Compute blob_kzg_commitments_root from the blobs bundle.
	// This is hash_tree_root(List[KZGCommitment, MAX_BLOB_COMMITMENTS_PER_BLOCK]).
	kzgCommitmentsRoot := make([]byte, 32)
	if blobsBundler != nil {
		commitments := extractKzgCommitments(blobsBundler)
		if len(commitments) > 0 {
			leaves := consensusblocks.LeavesFromCommitments(commitments)
			commitmentsTree, err := trie.GenerateTrieFromItems(leaves, fieldparams.LogMaxBlobCommitments)
			if err != nil {
				return nil, errors.Wrap(err, "could not generate kzg commitments trie")
			}
			root, err := commitmentsTree.HashTreeRoot()
			if err != nil {
				return nil, errors.Wrap(err, "could not compute kzg commitments root")
			}
			kzgCommitmentsRoot = root[:]
		}
	}

	return &ethpb.ExecutionPayloadBid{
		ParentBlockHash:        executionData.ParentHash(),
		ParentBlockRoot:        bytesutil.SafeCopyBytes(parentBlockRoot),
		BlockHash:              executionData.BlockHash(),
		PrevRandao:             executionData.PrevRandao(),
		FeeRecipient:           executionData.FeeRecipient(),
		GasLimit:               executionData.GasLimit(),
		BuilderIndex:           builderIndex,
		Slot:                   slot,
		Value:                  0, // Self-building: no competitive bid
		ExecutionPayment:       0, // Self-building: no payment
		BlobKzgCommitmentsRoot: kzgCommitmentsRoot,
	}, nil
}

// createExecutionPayloadEnvelope wraps a full execution payload with metadata.
// The envelope is cached by the beacon node during block production for later
// retrieval by the validator via GetExecutionPayloadEnvelope.
func (vs *Server) createExecutionPayloadEnvelope(
	executionData interfaces.ExecutionData,
	executionRequests *enginev1.ExecutionRequests,
	builderIndex primitives.BuilderIndex,
	slot primitives.Slot,
	blobsBundler enginev1.BlobsBundler,
) *ethpb.ExecutionPayloadEnvelope {
	// Extract the underlying ExecutionPayloadDeneb proto
	var payload *enginev1.ExecutionPayloadDeneb
	if executionData != nil && !executionData.IsNil() {
		if p, ok := executionData.Proto().(*enginev1.ExecutionPayloadDeneb); ok {
			payload = p
		}
	}

	commitments := extractKzgCommitments(blobsBundler)

	return &ethpb.ExecutionPayloadEnvelope{
		Payload:            payload,
		ExecutionRequests:  executionRequests,
		BuilderIndex:       builderIndex,
		BeaconBlockRoot:    make([]byte, 32), // Populated later when block root is known
		Slot:               slot,
		BlobKzgCommitments: commitments,
		StateRoot:          make([]byte, 32), // Computed later in GetExecutionPayloadEnvelope
	}
}

// extractKzgCommitments pulls KZG commitments from a blobs bundler.
func extractKzgCommitments(blobsBundler enginev1.BlobsBundler) [][]byte {
	if blobsBundler == nil {
		return nil
	}
	switch b := blobsBundler.(type) {
	case *enginev1.BlobsBundle:
		if b != nil {
			return b.KzgCommitments
		}
	case *enginev1.BlobsBundleV2:
		if b != nil {
			return b.KzgCommitments
		}
	}
	return nil
}

// cacheExecutionPayloadEnvelope stores an envelope for later retrieval by the validator.
func (vs *Server) cacheExecutionPayloadEnvelope(envelope *ethpb.ExecutionPayloadEnvelope) {
	if vs.ExecutionPayloadEnvelopeCache == nil {
		log.Warn("ExecutionPayloadEnvelopeCache is nil, envelope will not be cached")
		return
	}
	vs.ExecutionPayloadEnvelopeCache.Set(envelope)
}

// GetExecutionPayloadEnvelope retrieves a cached execution payload envelope.
// This is called by validators after receiving a GLOAS block to get the envelopeF
// they need to sign and broadcast.
//
// gRPC endpoint: /eth/v1alpha1/validator/execution_payload_envelope/{slot}/{builder_index}
func (vs *Server) GetExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.ExecutionPayloadEnvelopeRequest,
) (*ethpb.ExecutionPayloadEnvelopeResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if slots.ToEpoch(req.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload envelopes are not supported before GLOAS fork (slot %d)", req.Slot)
	}

	if vs.ExecutionPayloadEnvelopeCache == nil {
		return nil, status.Error(codes.Internal, "execution payload envelope cache not initialized")
	}

	envelope, found := vs.ExecutionPayloadEnvelopeCache.Get(req.Slot, req.BuilderIndex)
	if !found {
		return nil, status.Errorf(
			codes.NotFound,
			"execution payload envelope not found for slot %d builder %d",
			req.Slot,
			req.BuilderIndex,
		)
	}

	// Compute state root if not already set.
	// Following the pattern from epbs-interop: compute post-payload state root.
	if len(envelope.StateRoot) == 0 || bytesutil.ZeroRoot(envelope.StateRoot) {
		stateRoot, err := vs.computePostPayloadStateRoot(ctx, envelope)
		if err != nil {
			log.WithError(err).Warn("Failed to compute post-payload state root")
		} else {
			envelope.StateRoot = stateRoot
			log.WithField("stateRoot", fmt.Sprintf("%#x", stateRoot)).Debug("Computed state root at execution stage")
		}
	}

	return &ethpb.ExecutionPayloadEnvelopeResponse{
		Envelope: envelope,
	}, nil
}

// computePostPayloadStateRoot computes the state root after an execution
// payload envelope has been processed through a state transition.
func (vs *Server) computePostPayloadStateRoot(ctx context.Context, envelope *ethpb.ExecutionPayloadEnvelope) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.computePostPayloadStateRoot")
	defer span.End()

	if len(envelope.BeaconBlockRoot) == 0 || bytesutil.ZeroRoot(envelope.BeaconBlockRoot) {
		return nil, errors.New("beacon block root not set on envelope")
	}

	blockRoot := bytesutil.ToBytes32(envelope.BeaconBlockRoot)
	st, err := vs.StateGen.StateByRoot(ctx, blockRoot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get state by block root")
	}
	if st == nil {
		return nil, errors.New("nil state for block root")
	}

	// Copy the state to avoid mutating the original
	st = st.Copy()

	// TODO: Process the execution payload envelope through state transition.
	// This requires implementing ProcessPayloadStateTransition in beacon-chain/core/gloas.
	// For now, use the state root from the beacon block state as a placeholder.
	// The correct implementation would:
	// 1. Call ProcessPayloadStateTransition(ctx, st, envelope) to apply payload effects
	// 2. Compute HashTreeRoot of the resulting state

	root, err := st.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute state root")
	}
	return root[:], nil
}

// envelopeBlockWaitTimeout is the maximum time to wait for the associated beacon block
// before giving up on publishing the execution payload envelope.
const envelopeBlockWaitTimeout = 4 * time.Second

// envelopeBlockPollInterval is how often to check for the beacon block while waiting.
const envelopeBlockPollInterval = 100 * time.Millisecond

// PublishExecutionPayloadEnvelope validates and broadcasts a signed execution payload envelope.
// This is called by validators after signing the envelope retrieved from GetExecutionPayloadEnvelope.
//
// The function waits for the associated beacon block to be available before processing,
// as the envelope references a beacon_block_root that must exist either from local
// production or P2P gossip.
//
// gRPC endpoint: POST /eth/v1alpha1/validator/execution_payload_envelope
func (vs *Server) PublishExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.SignedExecutionPayloadEnvelope,
) (*emptypb.Empty, error) {
	if req == nil || req.Message == nil {
		return nil, status.Error(codes.InvalidArgument, "signed envelope cannot be nil")
	}

	if slots.ToEpoch(req.Message.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload envelopes are not supported before GLOAS fork (slot %d)", req.Message.Slot)
	}

	beaconBlockRoot := bytesutil.ToBytes32(req.Message.BeaconBlockRoot)

	log := log.WithFields(logrus.Fields{
		"slot":            req.Message.Slot,
		"builderIndex":    req.Message.BuilderIndex,
		"beaconBlockRoot": fmt.Sprintf("%#x", beaconBlockRoot[:8]),
	})
	log.Info("Publishing signed execution payload envelope")

	// Wait for the associated beacon block to be available.
	// The block may come from local production or P2P gossip.
	if err := vs.waitForBeaconBlock(ctx, beaconBlockRoot); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition,
			"beacon block %#x not available: %v", beaconBlockRoot[:8], err)
	}

	// TODO: Validate envelope signature before broadcasting
	// if err := vs.validateEnvelopeSignature(ctx, req); err != nil {
	//     return nil, status.Errorf(codes.InvalidArgument, "invalid envelope signature: %v", err)
	// }

	// Broadcast to P2P network
	if err := vs.P2P.Broadcast(ctx, req); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to broadcast signed execution payload envelope: %v", err)
	}

	// TODO: Receive the envelope locally following the broadcastReceiveBlock pattern.
	// This requires:
	// 1. blocks.WrappedROSignedExecutionPayloadEnvelope wrapper
	// 2. BlockReceiver.ReceiveExecutionPayloadEnvelope method
	// See epbs branch's receive_execution_payload_envelope.go for reference.

	log.Info("Successfully published execution payload envelope")

	return &emptypb.Empty{}, nil
}

// waitForBeaconBlock waits for the beacon block with the given root to be available.
// It first checks if the block already exists, then subscribes to block notifications
// and polls periodically until the block arrives or the timeout is reached.
func (vs *Server) waitForBeaconBlock(ctx context.Context, blockRoot [32]byte) error {
	// Fast path: check if block already exists
	if vs.BlockReceiver.HasBlock(ctx, blockRoot) {
		return nil
	}

	log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot[:8])).
		Debug("Waiting for beacon block to arrive")

	waitCtx, cancel := context.WithTimeout(ctx, envelopeBlockWaitTimeout)
	defer cancel()

	blocksChan := make(chan *feed.Event, 1)
	blockSub := vs.BlockNotifier.BlockFeed().Subscribe(blocksChan)
	defer blockSub.Unsubscribe()

	ticker := time.NewTicker(envelopeBlockPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return errors.Wrap(waitCtx.Err(), "timeout waiting for beacon block")

		case blockEvent := <-blocksChan:
			if blockEvent.Type == blockfeed.ReceivedBlock {
				data, ok := blockEvent.Data.(*blockfeed.ReceivedBlockData)
				if ok && data != nil && data.SignedBlock != nil {
					root, err := data.SignedBlock.Block().HashTreeRoot()
					if err == nil && root == blockRoot {
						return nil
					}
				}
			}

		case <-ticker.C:
			if vs.BlockReceiver.HasBlock(ctx, blockRoot) {
				return nil
			}

		case <-blockSub.Err():
			return errors.New("block subscription closed")
		}
	}
}
