package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/sirupsen/logrus"
)

// executionProofsByRootRPCHandler handles incoming ExecutionProofsByRoot RPC requests.
func (s *Service) executionProofsByRootRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.executionProofsByRootRPCHandler")
	defer span.End()

	_, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	req, ok := msg.(*ethpb.ExecutionProofsByRootRequest)
	if !ok {
		return errors.New("message is not type ExecutionProofsByRootRequest")
	}

	remotePeer := stream.Conn().RemotePeer()
	SetRPCStreamDeadlines(stream)

	// Validate request
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		return err
	}

	blockRoot := bytesutil.ToBytes32(req.BlockRoot)

	log := log.WithFields(logrus.Fields{
		"blockRoot": fmt.Sprintf("%#x", blockRoot),
		"peer":      remotePeer.String(),
	})

	s.rateLimiter.add(stream, 1)
	defer closeStream(stream, log)

	// Retrieve the slot corresponding to the block root.
	roSignedBeaconBlock, err := s.cfg.beaconDB.Block(ctx, blockRoot)
	if err != nil {
		return fmt.Errorf("fetch block from db: %w", err)
	}

	if roSignedBeaconBlock == nil {
		return fmt.Errorf("block not found for root %#x", blockRoot)
	}

	roBeaconBlock := roSignedBeaconBlock.Block()
	if roBeaconBlock == nil {
		return fmt.Errorf("beacon block is nil for root %#x", blockRoot)
	}

	slot := roBeaconBlock.Slot()

	// Get proofs from execution proof pool
	summary := s.cfg.proofStorage.Summary(blockRoot)
	if summary.Count() == 0 {
		return nil
	}

	// Load all proofs at once
	proofs, err := s.cfg.proofStorage.Get(blockRoot, nil)
	if err != nil {
		return fmt.Errorf("proof storage get: %w", err)
	}

	// Send proofs
	for _, proof := range proofs {
		// Write proof to stream
		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if err := WriteExecutionProofChunk(stream, s.cfg.p2p.Encoding(), slot, proof); err != nil {
			log.WithError(err).Debug("Could not send execution proof")
			s.writeErrorResponseToStream(responseCodeServerError, "could not send execution proof", stream)
			return err
		}
	}

	log.WithField("proofCount", len(proofs)).Debug("Responded to execution proofs by root request")

	return nil
}
