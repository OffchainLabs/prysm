package sync

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	prysmTrace "github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
)

// PayloadProcessor defines a block processing function, which allows to start utilizing
// payloads even before all payloads are ready.
type PayloadProcessor func(signed *enginev1.SignedExecutionPayloadEnvelope) error

// sendPayloadRequest sends a recent payload request to a peer to get
// those corresponding blocks from that peer.
func (s *Service) sendPayloadRequest(ctx context.Context, requests *types.BeaconBlockByRootsReq, id peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()

	requestedRoots := make(map[[32]byte]struct{})
	for _, root := range *requests {
		requestedRoots[root] = struct{}{}
	}

	return SendPayloadsByRootRequest(ctx, s.cfg.clock, s.cfg.p2p, id, requests, func(signed *enginev1.SignedExecutionPayloadEnvelope) error {
		if signed == nil || signed.Message == nil || signed.Message.BeaconBlockRoot == nil {
			return errors.New("received invalid nil payload")
		}
		root := [32]byte(signed.Message.BeaconBlockRoot)
		if _, ok := requestedRoots[root]; !ok {
			return fmt.Errorf("received unexpected payload with block root %#x", root)
		}
		s.pendingExecutionPayloads.Add(signed)
		if s.cfg.chain.InForkchoice(root) {
			s.processPendingPayloads(root)
		}
		return nil
	})
}

func ReadChunkedPayload(stream libp2pcore.Stream, tor blockchain.TemporalOracle, p2p p2p.EncodingProvider, isFirstChunk bool) (*enginev1.SignedExecutionPayloadEnvelope, error) {
	// Handle deadlines differently for first chunk
	if isFirstChunk {
		return readFirstChunkedPayload(stream, tor, p2p)
	}

	return readResponsePayloadChunk(stream, tor, p2p)
}

func readFirstChunkedPayload(stream libp2pcore.Stream, tor blockchain.TemporalOracle, p2p p2p.EncodingProvider) (*enginev1.SignedExecutionPayloadEnvelope, error) {
	code, errMsg, err := ReadStatusCode(stream, p2p.Encoding())
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New(errMsg)
	}
	rpcCtx, err := readContextFromStream(stream)
	if err != nil {
		return nil, err
	}
	signed, err := extractDataTypeFromTypeMap(types.SignedExecutionPayloadEnvelopeMap, rpcCtx, tor)
	if err != nil {
		return nil, err
	}
	err = p2p.Encoding().DecodeWithMaxLength(stream, signed)
	return signed, err
}

func readResponsePayloadChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, p2p p2p.EncodingProvider) (*enginev1.SignedExecutionPayloadEnvelope, error) {
	SetStreamReadDeadline(stream, respTimeout)
	code, errMsg, err := readStatusCodeNoDeadline(stream, p2p.Encoding())
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New(errMsg)
	}
	// No-op for now with the rpc context.
	rpcCtx, err := readContextFromStream(stream)
	if err != nil {
		return nil, err
	}
	signed, err := extractDataTypeFromTypeMap(types.SignedExecutionPayloadEnvelopeMap, rpcCtx, tor)
	if err != nil {
		return nil, err
	}
	err = p2p.Encoding().DecodeWithMaxLength(stream, signed)
	return signed, err
}

// SendPayloadsByRootRequest sends PayloadByRoot and returns fetched payloads, if any.
func SendPayloadsByRootRequest(
	ctx context.Context, clock blockchain.TemporalOracle, p2pProvider p2p.P2P, pid peer.ID,
	req *p2ptypes.BeaconBlockByRootsReq, processor PayloadProcessor,
) error {
	topic, err := p2p.TopicFromMessage(p2p.ExecutionPayloadsByRootMessageName, slots.ToEpoch(clock.CurrentSlot()))
	if err != nil {
		return err
	}
	stream, err := p2pProvider.Send(ctx, req, topic, pid)
	if err != nil {
		return err
	}
	defer closeStream(stream, log)

	// Augment block processing function, if non-nil block processor is provided.
	payloads := make([]*enginev1.SignedExecutionPayloadEnvelope, 0, len(*req))
	process := func(signed *enginev1.SignedExecutionPayloadEnvelope) error {
		payloads = append(payloads, signed)
		if processor != nil {
			return processor(signed)
		}
		return nil
	}
	currentEpoch := slots.ToEpoch(clock.CurrentSlot())
	for i := 0; i < len(*req); i++ {
		// Exit if peer sends more than max request blocks.
		if uint64(i) >= params.MaxRequestBlock(currentEpoch) {
			break
		}
		isFirstChunk := i == 0
		signed, err := ReadChunkedPayload(stream, clock, p2pProvider, isFirstChunk)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		if err := process(signed); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) sendBatchPayloadRequest(ctx context.Context, roots [][32]byte, randGen *rand.Rand) error {
	ctx, span := prysmTrace.StartSpan(ctx, "sendBatchPayloadRequest")
	defer span.End()

	roots = dedupRoots(roots)
	for i := len(roots) - 1; i >= 0; i-- {
		r := roots[i]
		if s.pendingExecutionPayloads.Has(r) || s.cfg.chain.PayloadBeingSynced(r) {
			roots = append(roots[:i], roots[i+1:]...)
		} else {
			log.WithField("blockRoot", fmt.Sprintf("%#x", r)).Debug("Requesting payload by root")
		}
	}

	if len(roots) == 0 {
		return nil
	}
	bestPeers := s.getBestPeers()
	if len(bestPeers) == 0 {
		return nil
	}
	// Randomly choose a peer to query from our best peers. If that peer cannot return
	// all the requested blocks, we randomly select another peer.
	pid := bestPeers[randGen.Int()%len(bestPeers)]
	for i := 0; i < numOfTries; i++ {
		req := p2ptypes.BeaconBlockByRootsReq(roots)
		currentEpoch := slots.ToEpoch(s.cfg.clock.CurrentSlot())
		maxReqBlock := params.MaxRequestBlock(currentEpoch)
		if uint64(len(roots)) > maxReqBlock {
			req = roots[:maxReqBlock]
		}
		if err := s.sendPayloadRequest(ctx, &req, pid); err != nil {
			tracing.AnnotateError(span, err)
			log.WithError(err).Debug("Could not send recent payload request")
		}
		newRoots := make([][32]byte, 0, len(roots))
		s.pendingQueueLock.RLock()
		for _, rt := range roots {
			if !s.seenPendingBlocks[rt] {
				newRoots = append(newRoots, rt)
			}
		}
		s.pendingQueueLock.RUnlock()
		if len(newRoots) == 0 {
			break
		}
		// Choosing a new peer with the leftover set of
		// roots to request.
		roots = newRoots
		pid = bestPeers[randGen.Int()%len(bestPeers)]
	}
	return nil
}
