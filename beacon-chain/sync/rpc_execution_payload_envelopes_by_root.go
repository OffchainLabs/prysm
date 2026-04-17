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
	recordResult := func(result executionPayloadEnvelopeRPCResult) {
		gloasExecutionPayloadEnvelopesRPCRequestsTotal.WithLabelValues("by_root", string(result)).Inc()
		if result == executionPayloadEnvelopeRPCResultServed {
			syncPayloadEnvelopeByRootServedTotal.Inc()
		}
	}
	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.ExecutionPayloadEnvelopesByRootName[1:]) // slice the leading slash off the name var

	ref, ok := msg.(*types.ExecutionPayloadEnvelopesByRootReq)
	if !ok {
		recordResult(executionPayloadEnvelopeRPCResultInvalid)
		return errors.New("message is not type ExecutionPayloadEnvelopesByRootReq")
	}

	requestedRoots := *ref

	if err := s.rateLimiter.validateRequest(stream, uint64(len(requestedRoots))); err != nil {
		recordResult(executionPayloadEnvelopeRPCResultRateLimited)
		return errors.Wrap(err, "rate limiter validate request")
	}

	remotePeer := stream.Conn().RemotePeer()
	if err := validateExecutionPayloadEnvelopeByRootRequest(len(requestedRoots)); err != nil {
		recordResult(executionPayloadEnvelopeRPCResultInvalid)
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
		defer ticker.Stop()
	}

	defer closeStream(stream, log)

	type requestedEnvelope struct {
		root [32]byte
		env  *ethpb.SignedBlindedExecutionPayloadEnvelope
	}
	if s.cfg.executionReconstructor == nil {
		recordResult(executionPayloadEnvelopeRPCResultError)
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		return errors.New("execution reconstructor is nil")
	}

	for start := 0; start < len(requestedRoots); start += batchSize {
		if start != 0 && ticker != nil {
			<-ticker.C
		}
		end := min(start+batchSize, len(requestedRoots))
		rootsBatch := requestedRoots[start:end]

		requestedEnvs := make([]requestedEnvelope, 0, len(rootsBatch))
		batchHashes := make([][32]byte, 0, len(rootsBatch))
		hashSeen := make(map[[32]byte]struct{}, len(rootsBatch))

		for _, root := range rootsBatch {
			if err := ctx.Err(); err != nil {
				recordResult(executionPayloadEnvelopeRPCResultError)
				return err
			}
			s.rateLimiter.add(stream, 1)

			blindedEnvelope, err := s.cfg.beaconDB.ExecutionPayloadEnvelope(ctx, root)
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					log.WithField("root", fmt.Sprintf("%#x", root)).Trace("Peer requested execution payload envelope by root not found")
					continue
				}
				log.WithError(err).Debug("Could not fetch blinded execution payload envelope")
				recordResult(executionPayloadEnvelopeRPCResultError)
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
			continue
		}

		payloadByHash, batchErr := s.cfg.executionReconstructor.ReconstructFullGloasExecutionPayloadsByHash(ctx, batchHashes)
		if batchErr != nil {
			recordResult(executionPayloadEnvelopeRPCResultError)
			log.WithError(batchErr).Debug("Could not batch reconstruct full execution payload envelopes")
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			return batchErr
		}

		for _, req := range requestedEnvs {
			blockHash := bytesutil.ToBytes32(req.env.Message.BlockHash)

			payload := payloadByHash[blockHash]
			if payload == nil {
				log.WithField("root", fmt.Sprintf("%#x", req.root)).Debug("Missing reconstructed payload after successful batch call")
				continue
			}
			payload.SlotNumber = req.env.Message.Slot
			envelope := &ethpb.SignedExecutionPayloadEnvelope{
				Message: &ethpb.ExecutionPayloadEnvelope{
					Payload:           payload,
					ExecutionRequests: req.env.Message.ExecutionRequests,
					BuilderIndex:      req.env.Message.BuilderIndex,
					BeaconBlockRoot:   req.env.Message.BeaconBlockRoot,
					StateRoot:         req.env.Message.StateRoot,
				},
				Signature: req.env.Signature,
			}
			SetStreamWriteDeadline(stream, defaultWriteDuration)
			if chunkErr := WriteExecutionPayloadEnvelopeChunk(stream, s.cfg.p2p.Encoding(), envelope); chunkErr != nil {
				log.WithError(chunkErr).Debug("Could not send a chunked response")
				recordResult(executionPayloadEnvelopeRPCResultError)
				s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
				tracing.AnnotateError(span, chunkErr)
				return chunkErr
			}
		}
	}

	recordResult(executionPayloadEnvelopeRPCResultServed)
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
