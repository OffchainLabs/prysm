package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
)

// executionPayloadEnvelopesByRootRPCHandler handles the
// /eth2/beacon_chain/req/execution_payload_envelopes_by_root/1/ RPC request.
// spec: https://github.com/ethereum/consensus-specs/blob/master/specs/gloas/p2p-interface.md#executionpayloadenvelopesbyroot-v1
func (s *Service) executionPayloadEnvelopesByRootRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.executionPayloadEnvelopesByRootRPCHandler")
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.ExecutionPayloadEnvelopesByRootName[1:]) // slice the leading slash off the name var

	ref, ok := msg.(*types.ExecutionPayloadEnvelopesByRootReq)
	if !ok {
		return errors.New("message is not type ExecutionPayloadEnvelopesByRootReq")
	}

	requestedRoots := *ref

	if err := s.rateLimiter.validateRequest(stream, uint64(len(requestedRoots))); err != nil {
		return errors.Wrap(err, "rate limiter validate request")
	}

	remotePeer := stream.Conn().RemotePeer()
	if err := validateExecutionPayloadEnvelopeByRootRequest(len(requestedRoots)); err != nil {
		s.downscorePeer(remotePeer, "executionPayloadEnvelopesByRootRPCHandlerValidationError")
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		return err
	}

	// Compute the oldest slot we'll allow a peer to request, based on the finalized epoch.
	finalized := s.cfg.chain.FinalizedCheckpt()
	minReqSlot, err := slots.EpochStart(finalized.Epoch)
	if err != nil {
		return errors.Wrapf(err, "could not compute start slot for finalized epoch %d", finalized.Epoch)
	}

	batchSize := flags.Get().BlockBatchLimit
	var ticker *time.Ticker
	if len(requestedRoots) > batchSize {
		ticker = time.NewTicker(time.Second)
	}

	defer closeStream(stream, log)

	type requestedEnvelope struct {
		root [32]byte
		env  *ethpb.SignedBlindedExecutionPayloadEnvelope
	}
	requestedEnvs := make([]requestedEnvelope, 0, len(requestedRoots))
	batchHashes := make([][32]byte, 0, len(requestedRoots))
	hashSeen := make(map[[32]byte]struct{}, len(requestedRoots))
	if s.cfg.executionReconstructor == nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		return errors.New("execution reconstructor is nil")
	}

	for i, root := range requestedRoots {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Throttle request processing.
		if i != 0 && i%batchSize == 0 && ticker != nil {
			<-ticker.C
		}
		s.rateLimiter.add(stream, 1)

		blindedEnvelope, err := s.cfg.beaconDB.ExecutionPayloadEnvelope(ctx, root)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				log.WithField("root", fmt.Sprintf("%#x", root)).Trace("Peer requested execution payload envelope by root not found")
				continue
			}
			log.WithError(err).Debug("Could not fetch blinded execution payload envelope")
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			return err
		}
		if blindedEnvelope == nil || blindedEnvelope.Message == nil {
			continue
		}

		// Silently skip envelopes older than the finalized epoch.
		// The spec requires serving envelopes since the latest finalized epoch;
		// pre-finalization envelopes are omitted from the response rather than erroring.
		if blindedEnvelope.Message.Slot < minReqSlot {
			continue
		}

		requestedEnvs = append(requestedEnvs, requestedEnvelope{root: root, env: blindedEnvelope})
		blockHash := bytesutil.ToBytes32(blindedEnvelope.Message.BlockHash)
		if _, ok := hashSeen[blockHash]; !ok {
			hashSeen[blockHash] = struct{}{}
			batchHashes = append(batchHashes, blockHash)
		}
	}

	if len(requestedEnvs) == 0 {
		return nil
	}

	payloadByHash, batchErr := s.cfg.executionReconstructor.ReconstructFullExecutionPayloadsByHash(ctx, batchHashes)
	if batchErr != nil {
		log.WithError(batchErr).Debug("Could not batch reconstruct full execution payload envelopes")
		payloadByHash = nil
	}

	for i := range requestedEnvs {
		req := requestedEnvs[i]
		blockHash := bytesutil.ToBytes32(req.env.Message.BlockHash)

		payload := payloadByHash[blockHash]
		if payload == nil {
			payload, err = s.cfg.executionReconstructor.ReconstructFullExecutionPayloadByHash(ctx, blockHash)
			if err != nil {
				log.WithError(err).WithField("root", fmt.Sprintf("%#x", req.root)).Debug("Could not reconstruct full execution payload envelope")
				continue
			}
		}
		envelope := &ethpb.SignedExecutionPayloadEnvelope{
			Message: &ethpb.ExecutionPayloadEnvelope{
				Payload:           payload,
				ExecutionRequests: req.env.Message.ExecutionRequests,
				BuilderIndex:      req.env.Message.BuilderIndex,
				BeaconBlockRoot:   req.env.Message.BeaconBlockRoot,
				Slot:              req.env.Message.Slot,
				StateRoot:         req.env.Message.StateRoot,
			},
			Signature: req.env.Signature,
		}
		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if chunkErr := WriteExecutionPayloadEnvelopeChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), envelope); chunkErr != nil {
			log.WithError(chunkErr).Debug("Could not send a chunked response")
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			tracing.AnnotateError(span, chunkErr)
			return chunkErr
		}
	}

	return nil
}

// validateExecutionPayloadEnvelopeByRootRequest checks if the request for execution payload envelopes is valid.
func validateExecutionPayloadEnvelopeByRootRequest(count int) error {
	if count == 0 {
		return types.ErrInvalidRequest
	}
	if uint64(count) > params.BeaconConfig().MaxRequestPayloads {
		return types.ErrMaxPayloadEnvelopeReqExceeded
	}
	return nil
}
