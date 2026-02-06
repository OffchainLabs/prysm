package validator

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	blockfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/block"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GloasBlockBuilder defines the interface for building GLOAS beacon blocks.
// This interface allows for easier testing and potential alternative implementations.
type GloasBlockBuilder interface {
	// BuildGloasBlock constructs a GLOAS beacon block for the given slot.
	// Returns the block with a signed execution payload bid and caches the
	// corresponding execution payload envelope for later retrieval.
	BuildGloasBlock(ctx context.Context, slot primitives.Slot, randaoReveal, graffiti []byte) (*ethpb.BeaconBlockGloas, error)
}

// GloasEnvelopePublisher defines the interface for broadcasting execution payload envelopes.
type GloasEnvelopePublisher interface {
	// PublishExecutionPayloadEnvelope broadcasts a signed execution payload envelope
	// to the P2P network after validating its signature.
	PublishExecutionPayloadEnvelope(ctx context.Context, envelope *ethpb.SignedExecutionPayloadEnvelope) error
}

// getGloasBeaconBlock produces a GLOAS beacon block for the given request.
// This is called from GetBeaconBlock when the requested slot is in the GLOAS fork.
//
// The GLOAS flow differs from previous forks:
// 1. Block contains a signed execution payload bid instead of full payload
// 2. Execution payload envelope is cached for later retrieval
// 3. Validator must separately retrieve, sign, and broadcast the envelope
func (vs *Server) getGloasBeaconBlock(ctx context.Context, req *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error) {
	// TODO: Implement GLOAS block production
	//
	// Implementation steps:
	// 1. Get parent state via getParentState()
	// 2. Create empty GLOAS block template
	// 3. Set basic fields (slot, proposer_index, parent_root, graffiti, randao_reveal)
	// 4. Build consensus fields via BuildBlockParallel() - reuse existing logic
	// 5. Get execution payload from local execution client
	// 6. Create execution payload bid from the payload
	// 7. Sign the bid (for self-building, proposer signs as builder)
	// 8. Create execution payload envelope and cache it
	// 9. Return GenericBeaconBlock with GLOAS block type

	return nil, status.Error(codes.Unimplemented, "GLOAS block production not yet implemented")
}

// buildGloasBlock constructs a GLOAS beacon block with all required fields.
func (vs *Server) buildGloasBlock(ctx context.Context, slot primitives.Slot, randaoReveal, graffiti []byte) (*ethpb.BeaconBlockGloas, error) {
	// TODO: Implement GLOAS block building
	//
	// This method should:
	// 1. Reuse BuildBlockParallel() for consensus fields (attestations, slashings, etc.)
	// 2. Create the execution payload bid instead of including full payload
	// 3. Include payload attestations from the previous slot's PTC

	return nil, errors.New("buildGloasBlock not yet implemented")
}

// createExecutionPayloadBid creates an ExecutionPayloadBid from a full execution payload.
// For local block building, the beacon node acts as its own builder.
func (vs *Server) createExecutionPayloadBid(
	ctx context.Context,
	slot primitives.Slot,
	builderIndex primitives.BuilderIndex,
	parentBlockHash []byte,
	parentBlockRoot []byte,
	payload any, // TODO: Use proper execution payload type
) (*ethpb.ExecutionPayloadBid, error) {
	// TODO: Implement bid creation
	//
	// The bid should contain:
	// - parent_block_hash: From execution payload
	// - parent_block_root: From beacon chain
	// - block_hash: Execution payload block hash
	// - prev_randao: From beacon state
	// - fee_recipient: From execution payload
	// - gas_limit: From execution payload
	// - builder_index: The proposer's builder index (for self-building)
	// - slot: Current slot
	// - value: Bid value (for self-building, can be 0 or calculated)
	// - execution_payment: Payment amount
	// - blob_kzg_commitments_root: Hash tree root of blob commitments

	return nil, errors.New("createExecutionPayloadBid not yet implemented")
}

// signExecutionPayloadBid signs an execution payload bid.
// For local block building, this uses the proposer's key.
func (vs *Server) signExecutionPayloadBid(
	ctx context.Context,
	bid *ethpb.ExecutionPayloadBid,
	proposerIndex primitives.ValidatorIndex,
) (*ethpb.SignedExecutionPayloadBid, error) {
	// TODO: Implement bid signing
	//
	// For local/self-building:
	// - The proposer acts as the builder
	// - Sign with the proposer's key
	// - Use DOMAIN_EXECUTION_PAYLOAD_BID signing domain

	return nil, errors.New("signExecutionPayloadBid not yet implemented")
}

// createExecutionPayloadEnvelope wraps an execution payload with metadata for the envelope.
func (vs *Server) createExecutionPayloadEnvelope(
	ctx context.Context,
	payload any, // TODO: Use proper execution payload type
	executionRequests any, // TODO: Use proper type
	builderIndex primitives.BuilderIndex,
	beaconBlockRoot []byte,
	slot primitives.Slot,
	blobKzgCommitments [][]byte,
	stateRoot []byte,
) (*ethpb.ExecutionPayloadEnvelope, error) {
	// TODO: Implement envelope creation
	//
	// The envelope wraps the full execution payload with:
	// - payload: The full execution payload
	// - execution_requests: EL execution requests
	// - builder_index: Builder who created this payload
	// - beacon_block_root: Root of the beacon block this envelope is for
	// - slot: Current slot
	// - blob_kzg_commitments: KZG commitments for blobs
	// - state_root: Beacon state root after applying the block

	return nil, errors.New("createExecutionPayloadEnvelope not yet implemented")
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
// This is called by validators after receiving a GLOAS block to get the envelope
// they need to sign and broadcast.
//
// gRPC endpoint: /eth/v1alpha1/validator/execution_payload_envelope/{slot}/{builder_index}
func (vs *Server) GetExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.ExecutionPayloadEnvelopeRequest,
) (*ethpb.ExecutionPayloadEnvelopeResponse, error) {
	// Validate request
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Check if cache is available
	if vs.ExecutionPayloadEnvelopeCache == nil {
		return nil, status.Error(codes.Internal, "execution payload envelope cache not initialized")
	}

	// Retrieve from cache
	envelope, found := vs.ExecutionPayloadEnvelopeCache.Get(req.Slot, req.BuilderIndex)
	if !found {
		return nil, status.Errorf(
			codes.NotFound,
			"execution payload envelope not found for slot %d builder %d",
			req.Slot,
			req.BuilderIndex,
		)
	}

	// Compute state root if not already set
	// Following the pattern from epbs-interop: compute post-payload state root
	if len(envelope.StateRoot) == 0 || bytesutil.ZeroRoot(envelope.StateRoot) {
		stateRoot, err := vs.computePostPayloadStateRoot(ctx, envelope)
		if err != nil {
			log.WithError(err).Warn("Failed to compute post-payload state root")
			// Continue without state root - validator may still need the envelope
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
// This follows the pattern from epbs-interop.
func (vs *Server) computePostPayloadStateRoot(ctx context.Context, envelope *ethpb.ExecutionPayloadEnvelope) ([]byte, error) {
	// TODO: Implement post-payload state root computation
	//
	// Steps from epbs-interop:
	// 1. Get beacon state by the envelope's beacon_block_root
	// 2. Copy the state
	// 3. Call UpdateHeaderAndVerify to verify the header
	// 4. Call ProcessPayloadStateTransition to process the envelope
	// 5. Compute and return the state root

	return nil, errors.New("computePostPayloadStateRoot not yet implemented")
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

	// TODO: Wrap and receive the envelope locally
	// This requires implementing blocks.WrappedROSignedExecutionPayloadEnvelope
	// in the consensus-types/blocks package, similar to epbs-interop branch.
	//
	// For local processing, clear blob commitments to avoid duplicates:
	// reqCopy := &ethpb.SignedExecutionPayloadEnvelope{
	//     Message: &ethpb.ExecutionPayloadEnvelope{
	//         Payload:            req.Message.Payload,
	//         ExecutionRequests:  req.Message.ExecutionRequests,
	//         BuilderIndex:       req.Message.BuilderIndex,
	//         BeaconBlockRoot:    req.Message.BeaconBlockRoot,
	//         Slot:               req.Message.Slot,
	//         BlobKzgCommitments: [][]byte{}, // Clear for local processing
	//         StateRoot:          req.Message.StateRoot,
	//     },
	//     Signature: req.Signature,
	// }
	//
	// wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(reqCopy)
	// if err != nil {
	//     return nil, status.Errorf(codes.InvalidArgument, "failed to wrap execution payload envelope: %v", err)
	// }
	//
	// Also requires adding ExecutionPayloadReceiver to the server struct:
	// if err := vs.ExecutionPayloadReceiver.ReceiveExecutionPayloadEnvelope(ctx, wrapped, nil); err != nil {
	//     return nil, status.Errorf(codes.Internal, "failed to receive execution payload envelope: %v", err)
	// }

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

	// Create a context with timeout for waiting
	waitCtx, cancel := context.WithTimeout(ctx, envelopeBlockWaitTimeout)
	defer cancel()

	// Subscribe to block notifications
	blocksChan := make(chan *feed.Event, 1)
	blockSub := vs.BlockNotifier.BlockFeed().Subscribe(blocksChan)
	defer blockSub.Unsubscribe()

	// Create a ticker for periodic polling
	ticker := time.NewTicker(envelopeBlockPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return errors.Wrap(waitCtx.Err(), "timeout waiting for beacon block")

		case blockEvent := <-blocksChan:
			// Check if this is the block we're waiting for
			if blockEvent.Type == blockfeed.ReceivedBlock {
				data, ok := blockEvent.Data.(*blockfeed.ReceivedBlockData)
				if ok && data != nil && data.SignedBlock != nil {
					receivedRoot := data.SignedBlock.Block().HashTreeRoot
					root, err := receivedRoot()
					if err == nil && root == blockRoot {
						log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot[:8])).
							Debug("Received beacon block via notification")
						return nil
					}
				}
			}

		case <-ticker.C:
			// Periodic poll in case we missed a notification
			if vs.BlockReceiver.HasBlock(ctx, blockRoot) {
				log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot[:8])).
					Debug("Found beacon block via polling")
				return nil
			}

		case <-blockSub.Err():
			return errors.New("block subscription closed")
		}
	}
}

// validateEnvelopeSignature verifies the signature on a signed execution payload envelope.
func (vs *Server) validateEnvelopeSignature(
	ctx context.Context,
	signedEnvelope *ethpb.SignedExecutionPayloadEnvelope,
) error {
	// TODO: Implement signature validation
	//
	// Steps:
	// 1. Get head state
	// 2. Look up builder in builder registry by builder_index
	// 3. Get builder's pubkey
	// 4. Compute signing root for envelope using DOMAIN_EXECUTION_PAYLOAD_ENVELOPE
	// 5. Verify BLS signature

	return errors.New("validateEnvelopeSignature not yet implemented")
}

// TODO: The following wrapper function needs to be added to consensus-types/blocks:
// - WrappedROSignedExecutionPayloadEnvelope(envelope *ethpb.SignedExecutionPayloadEnvelope) (interfaces.ROSignedExecutionPayloadEnvelope, error)
// - WrappedROExecutionPayloadEnvelope(envelope *ethpb.ExecutionPayloadEnvelope) (interfaces.ROExecutionPayloadEnvelope, error)
//
// These are needed to properly receive and process execution payload envelopes.
// See the epbs-interop branch for reference implementation.
