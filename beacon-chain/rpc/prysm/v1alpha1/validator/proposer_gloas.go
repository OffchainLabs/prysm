package validator

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	blockfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/block"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// setGloasExecutionData creates an execution payload bid from the local payload
// and sets it on the block body. The envelope is created and cached later by
// buildExecutionPayloadEnvelope once the block is fully built.
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
	bid, err := vs.createSelfBuildExecutionPayloadBid(
		local,
		primitives.BuilderIndex(sBlk.Block().ProposerIndex()),
		parentRoot[:],
		sBlk.Block().Slot(),
	)
	if err != nil {
		return errors.Wrap(err, "could not create execution payload bid")
	}

	// Per spec, self-build bids must use G2 point-at-infinity as the signature.
	// Only the execution payload envelope requires a real signature from the proposer.
	signedBid := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: common.InfiniteSignature[:],
	}
	if err := sBlk.SetSignedExecutionPayloadBid(signedBid); err != nil {
		return errors.Wrap(err, "could not set signed execution payload bid")
	}

	return nil
}

// buildExecutionPayloadEnvelope creates and caches the execution payload envelope
// after the block is fully built (state root set). This allows setting the
// BeaconBlockRoot from the final block HTR and computing the post-payload state root.
func (vs *Server) buildExecutionPayloadEnvelope(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	head state.BeaconState,
	local *consensusblocks.GetPayloadResponse,
) error {
	blockRoot, err := sBlk.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute block hash tree root")
	}

	// Extract the underlying ExecutionPayloadDeneb proto.
	var payload *enginev1.ExecutionPayloadDeneb
	if local.ExecutionData != nil && !local.ExecutionData.IsNil() {
		if p, ok := local.ExecutionData.Proto().(*enginev1.ExecutionPayloadDeneb); ok {
			payload = p
		}
	}

	// TODO: Compute post-payload state root. This requires:
	// 1. Run state transition on head state with the block to get post-block state
	// 2. Run ProcessPayloadStateTransition(ctx, postBlockState, envelope) to apply
	//    execution payload effects (deposits, withdrawals, consolidations, etc.)
	// 3. Set stateRoot = postPayloadState.HashTreeRoot()
	// For now, the state root remains zeroed until ProcessPayloadStateTransition
	// is implemented in beacon-chain/core/gloas.

	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload:           payload,
		ExecutionRequests: local.ExecutionRequests,
		BuilderIndex:      primitives.BuilderIndex(sBlk.Block().ProposerIndex()),
		BeaconBlockRoot:   blockRoot[:],
		Slot:              sBlk.Block().Slot(),
		StateRoot:         make([]byte, 32),
	}

	vs.cacheExecutionPayloadEnvelope(envelope, local.BlobsBundler)
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

// createSelfBuildExecutionPayloadBid creates an ExecutionPayloadBid for self-building,
// where the proposer acts as its own builder. The value is the block value from the
// execution layer, and execution payment is zero since the proposer doesn't pay themselves.
func (vs *Server) createSelfBuildExecutionPayloadBid(
	local *consensusblocks.GetPayloadResponse,
	builderIndex primitives.BuilderIndex,
	parentBlockRoot []byte,
	slot primitives.Slot,
) (*ethpb.ExecutionPayloadBid, error) {
	ed := local.ExecutionData
	if ed == nil || ed.IsNil() {
		return nil, errors.New("execution data is nil")
	}

	return &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    ed.ParentHash(),
		ParentBlockRoot:    bytesutil.SafeCopyBytes(parentBlockRoot),
		BlockHash:          ed.BlockHash(),
		PrevRandao:         ed.PrevRandao(),
		FeeRecipient:       ed.FeeRecipient(),
		GasLimit:           ed.GasLimit(),
		BuilderIndex:       builderIndex,
		Slot:               slot,
		Value:              primitives.WeiToGwei(local.Bid),
		ExecutionPayment:   0, // Self-build: proposer doesn't pay themselves.
		BlobKzgCommitments: extractKzgCommitments(local.BlobsBundler),
	}, nil
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

// cacheExecutionPayloadEnvelope stores an envelope and its blobs bundle for later retrieval.
// The blobs bundle is cached alongside the envelope because blobs from the EL are only
// held in memory until they are broadcast as sidecars during block proposal.
func (vs *Server) cacheExecutionPayloadEnvelope(envelope *ethpb.ExecutionPayloadEnvelope, blobsBundle enginev1.BlobsBundler) {
	if vs.ExecutionPayloadEnvelopeCache == nil {
		log.Warn("ExecutionPayloadEnvelopeCache is nil, envelope will not be cached")
		return
	}
	vs.ExecutionPayloadEnvelopeCache.Set(envelope, blobsBundle)
}

// GetExecutionPayloadEnvelope retrieves a cached execution payload envelope.
// This is called by validators after receiving a GLOAS block to get the envelope
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

	return &ethpb.ExecutionPayloadEnvelopeResponse{
		Envelope: envelope,
	}, nil
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

	// Build data column sidecars from the cached blobs bundle before broadcasting.
	// In GLOAS, blob data is delivered alongside the execution payload envelope
	// rather than with the beacon block (which only carries the bid).
	dataColumnSidecars, err := vs.buildEnvelopeDataColumns(ctx, req.Message, beaconBlockRoot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to build data column sidecars: %v", err)
	}

	// Broadcast envelope and data column sidecars concurrently.
	eg, eCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		if err := vs.P2P.Broadcast(eCtx, req); err != nil {
			return errors.Wrap(err, "broadcast signed execution payload envelope")
		}
		// TODO: Receive the envelope locally following the broadcastReceiveBlock pattern.
		// This requires:
		// 1. blocks.WrappedROSignedExecutionPayloadEnvelope wrapper
		// 2. BlockReceiver.ReceiveExecutionPayloadEnvelope method
		// See epbs branch's receive_execution_payload_envelope.go for reference.
		return nil
	})
	if len(dataColumnSidecars) > 0 {
		eg.Go(func() error {
			return vs.broadcastAndReceiveDataColumns(eCtx, dataColumnSidecars)
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to publish execution payload envelope: %v", err)
	}

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

// buildEnvelopeDataColumns retrieves the cached blobs bundle for the envelope's
// slot/builder and builds data column sidecars. Returns nil if no blobs to broadcast.
func (vs *Server) buildEnvelopeDataColumns(
	ctx context.Context,
	envelope *ethpb.ExecutionPayloadEnvelope,
	blockRoot [32]byte,
) ([]consensusblocks.RODataColumn, error) {
	if vs.ExecutionPayloadEnvelopeCache == nil {
		return nil, nil
	}

	blobsBundle, found := vs.ExecutionPayloadEnvelopeCache.GetBlobsBundle(envelope.Slot, envelope.BuilderIndex)
	if !found || blobsBundle == nil {
		return nil, nil
	}

	blobs := blobsBundle.GetBlobs()
	proofs := blobsBundle.GetProofs()
	commitments := blobsBundle.GetKzgCommitments()
	if len(blobs) == 0 {
		return nil, nil
	}

	// Retrieve the beacon block to build the signed block header for sidecars.
	blk, err := vs.BeaconDB.Block(ctx, blockRoot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get block for data column sidecars")
	}
	if blk == nil {
		return nil, errors.New("block not found for data column sidecars")
	}

	roBlock, err := consensusblocks.NewROBlockWithRoot(blk, blockRoot)
	if err != nil {
		return nil, errors.Wrap(err, "could not create ROBlock")
	}

	return buildDataColumnSidecars(blobs, proofs, peerdas.PopulateFromEnvelope(roBlock, commitments))
}
