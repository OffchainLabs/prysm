package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/sirupsen/logrus"
)

// executionProofsByRootRPCHandler handles incoming ExecutionProofsByRoot RPC requests.
// spec: https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8025/p2p-interface.md#executionproofsbyroot
func (s *Service) executionProofsByRootRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.executionProofsByRootRPCHandler")
	defer span.End()

	_, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	identifiers, ok := msg.(types.ExecutionProofsByRootReq)
	if !ok {
		return errors.New("message is not type ExecutionProofsByRootReq")
	}

	count := uint64(len(identifiers))

	// Validate request size.
	maxIdents := params.BeaconConfig().MaxRequestBlocksDeneb
	if count > maxIdents {
		s.writeErrorResponseToStream(responseCodeInvalidRequest, "request exceeds max identifiers", stream)
		return fmt.Errorf("request exceeds max identifiers: %d > %d", count, maxIdents)
	}

	remotePeer := stream.Conn().RemotePeer()
	SetRPCStreamDeadlines(stream)

	// Charge one rate-limit token per request.
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		return fmt.Errorf("validator request: %w", err)
	}

	s.rateLimiter.add(stream, 1)
	defer closeStream(stream, log)

	log := log.WithField("peer", remotePeer.String())

	requestedByRoot := make(map[string][]string, len(identifiers))
	for _, identifier := range identifiers {
		if identifier == nil {
			continue
		}
		rootKey := fmt.Sprintf("%#x", identifier.BlockRoot)
		names := make([]string, 0, len(identifier.ProofTypes))
		for _, pt := range identifier.ProofTypes {
			names = append(names, ethpb.ProofTypeName(pt))
		}
		requestedByRoot[rootKey] = names
	}
	log.WithFields(logrus.Fields{
		"blocks":          len(identifiers),
		"requestedByRoot": requestedByRoot,
	}).Debug("Received execution proofs by root request")

	respondedByRoot := make(map[string][]string, len(identifiers))
	for _, identifier := range identifiers {
		if identifier == nil {
			continue
		}

		blockRoot := bytesutil.ToBytes32(identifier.BlockRoot)
		log := log.WithFields(logrus.Fields{
			"blockRoot":  fmt.Sprintf("%#x", blockRoot),
			"proofTypes": identifier.ProofTypes,
		})

		// Fetch the requested proofs from storage, filtered by requested types.
		summary := s.cfg.proofStorage.Summary(blockRoot)
		if summary.Count() == 0 {
			continue
		}

		proofs, err := s.cfg.proofStorage.Get(blockRoot, identifier.ProofTypes)
		if err != nil {
			return fmt.Errorf("proof storage get: %w", err)
		}

		// Send each proof as a separate chunk.
		rootKey := fmt.Sprintf("%#x", blockRoot)
		sentNames := make([]string, 0, len(proofs))
		for _, proof := range proofs {
			SetStreamWriteDeadline(stream, defaultWriteDuration)
			if err := WriteExecutionProofChunk(stream, s.cfg.p2p.Encoding(), proof); err != nil {
				log.WithError(err).Debug("Could not send execution proof")
				s.writeErrorResponseToStream(responseCodeServerError, "could not send execution proof", stream)
				return err
			}
			if len(proof.Message.ProofType) == 1 {
				sentNames = append(sentNames, ethpb.ProofTypeName(proof.Message.ProofType[0]))
			}
		}
		respondedByRoot[rootKey] = sentNames

		log.WithField("proofCount", len(proofs)).Debug("Responded to execution proofs by root request")
	}

	log.WithField("respondedByRoot", respondedByRoot).Debug("Sent execution proofs by root response")

	return nil
}
