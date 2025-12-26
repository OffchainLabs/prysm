package sync

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// SendExecutionProofsByRootRequest sends ExecutionProofsByRoot request and returns fetched execution proofs, if any.
func SendExecutionProofsByRootRequest(
	ctx context.Context,
	clock blockchain.TemporalOracle,
	p2pProvider p2p.P2P,
	pid peer.ID,
	req *ethpb.ExecutionProofsByRootRequest,
) ([]*ethpb.ExecutionProof, error) {
	// Validate request
	if req.CountNeeded == 0 {
		return nil, errors.New("count_needed must be greater than 0")
	}

	topic, err := p2p.TopicFromMessage(p2p.ExecutionProofsByRootName, slots.ToEpoch(clock.CurrentSlot()))
	if err != nil {
		return nil, err
	}

	log.WithFields(logrus.Fields{
		"topic":      topic,
		"block_root": bytesutil.ToBytes32(req.BlockRoot),
		"count":      req.CountNeeded,
		"already":    len(req.AlreadyHave),
	}).Debug("Sending execution proofs by root request")

	stream, err := p2pProvider.Send(ctx, req, topic, pid)
	if err != nil {
		return nil, err
	}
	defer closeStream(stream, log)

	// Read execution proofs from stream
	proofs := make([]*ethpb.ExecutionProof, 0, req.CountNeeded)
	alreadyHaveSet := make(map[primitives.ExecutionProofId]struct{})
	for _, id := range req.AlreadyHave {
		alreadyHaveSet[id] = struct{}{}
	}

	for i := uint64(0); i < req.CountNeeded; i++ {
		isFirstChunk := i == 0
		proof, err := ReadChunkedExecutionProof(stream, p2pProvider, isFirstChunk)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		// Validate proof
		if err := validateExecutionProof(proof, req, alreadyHaveSet); err != nil {
			return nil, err
		}

		proofs = append(proofs, proof)
	}

	return proofs, nil
}

// ReadChunkedExecutionProof reads a chunked execution proof from the stream.
func ReadChunkedExecutionProof(
	stream libp2pcore.Stream,
	encoding p2p.EncodingProvider,
	isFirstChunk bool,
) (*ethpb.ExecutionProof, error) {
	// Read status code for each chunk (like data columns, not like blocks)
	code, errMsg, err := ReadStatusCode(stream, encoding.Encoding())
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New(errMsg)
	}

	// Read context bytes (fork digest)
	_, err = readContextFromStream(stream)
	if err != nil {
		return nil, fmt.Errorf("read context from stream: %w", err)
	}

	// Decode the proof
	proof := &ethpb.ExecutionProof{}
	if err := encoding.Encoding().DecodeWithMaxLength(stream, proof); err != nil {
		return nil, err
	}

	return proof, nil
}

// validateExecutionProof validates a received execution proof against the request.
func validateExecutionProof(
	proof *ethpb.ExecutionProof,
	req *ethpb.ExecutionProofsByRootRequest,
	alreadyHaveSet map[primitives.ExecutionProofId]struct{},
) error {
	// Check block root matches
	proofRoot := bytesutil.ToBytes32(proof.BlockRoot)
	reqRoot := bytesutil.ToBytes32(req.BlockRoot)
	if proofRoot != reqRoot {
		return fmt.Errorf("proof block root %#x does not match requested root %#x",
			proofRoot, reqRoot)
	}

	// Check we didn't already have this proof
	if _, ok := alreadyHaveSet[proof.ProofId]; ok {
		return fmt.Errorf("received proof we already have: proof_id=%d", proof.ProofId)
	}

	// Check proof ID is valid (within max range)
	if !proof.ProofId.IsValid() {
		return fmt.Errorf("invalid proof_id: %d", proof.ProofId)
	}

	return nil
}

// executionProofsByRootRPCHandler handles incoming ExecutionProofsByRoot RPC requests.
func (s *Service) executionProofsByRootRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.executionProofsByRootRPCHandler")
	defer span.End()
	_, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	log := log.WithField("handler", "execution_proofs_by_root")

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

	// Penalize peers that send invalid requests.
	if err := validateExecutionProofsByRootRequest(req); err != nil {
		s.downscorePeer(remotePeer, "executionProofsByRootRPCHandlerValidationError")
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		return fmt.Errorf("validate execution proofs by root request: %w", err)
	}

	blockRoot := bytesutil.ToBytes32(req.BlockRoot)

	log.WithFields(logrus.Fields{
		"block_root": blockRoot,
		"count":      req.CountNeeded,
		"already":    len(req.AlreadyHave),
	}).Debug("Received execution proofs by root request")

	s.rateLimiter.add(stream, 1)
	defer closeStream(stream, log)

	if !features.Get().EnableZkvm {
		log.Debug("Disabled zkVM mode; refusing to serve execution proofs by root request")
		return nil
	}

	// Get proofs from execution proof pool
	availableProofs := s.cfg.execProofPool.GetProofsForBlock(blockRoot)
	if len(availableProofs) == 0 {
		log.Debug("No execution proofs available for block")
		return nil
	}

	// Filter out already_have proofs
	alreadyHaveSet := make(map[primitives.ExecutionProofId]struct{})
	for _, id := range req.AlreadyHave {
		alreadyHaveSet[id] = struct{}{}
	}

	// Send proofs
	sentCount := uint64(0)
	for _, proof := range availableProofs {
		if sentCount >= req.CountNeeded {
			break
		}

		// Skip proofs the requester already has
		if _, ok := alreadyHaveSet[proof.ProofId]; ok {
			continue
		}

		// Write proof to stream
		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if err := WriteExecutionProofChunk(stream, s.cfg.p2p.Encoding(), proof); err != nil {
			log.WithError(err).Debug("Could not send execution proof")
			s.writeErrorResponseToStream(responseCodeServerError, "could not send execution proof", stream)
			return err
		}

		sentCount++
	}

	log.WithFields(logrus.Fields{
		"block_root": blockRoot,
		"sent":       sentCount,
		"requested":  req.CountNeeded,
	}).Debug("Sent execution proofs")

	return nil
}

func validateExecutionProofsByRootRequest(req *ethpb.ExecutionProofsByRootRequest) error {
	if req.CountNeeded == 0 {
		return errors.New("count_needed must be greater than 0")
	}
	return nil
}
