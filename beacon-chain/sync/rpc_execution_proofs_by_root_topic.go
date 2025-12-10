package sync

import (
	"context"
	"errors"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
)

// SendBeaconBlocksByRootRequest sends BeaconBlocksByRoot and returns fetched blocks, if any.
func SendExecutionProofByRootRequest(
	ctx context.Context, clock blockchain.TemporalOracle, p2pProvider p2p.P2P, pid peer.ID,
	req *p2ptypes.ExecutionProofByRootsReq, blockProcessor BeaconBlockProcessor,
) ([]eth.ExecutionProof, error) {
	topic, err := p2p.TopicFromMessage(p2p.ExecutionProofsByRootName, slots.ToEpoch(clock.CurrentSlot()))
	if err != nil {
		return nil, err
	}
	stream, err := p2pProvider.Send(ctx, req, topic, pid)
	if err != nil {
		return nil, err
	}
	defer closeStream(stream, log)

	// Augment block processing function, if non-nil block processor is provided.
	execution_proofs := make([]eth.ExecutionProof, 0, len(*req))
	// process := func(block interfaces.ReadOnlySignedBeaconBlock) error {
	// 	blocks = append(blocks, block)
	// 	if blockProcessor != nil {
	// 		return blockProcessor(block)
	// 	}
	// 	return nil
	// }
	// currentEpoch := slots.ToEpoch(clock.CurrentSlot())
	// for i := 0; i < len(*req); i++ {
	// 	isFirstChunk := i == 0
	// 	blk, err := ReadChunkedBlock(stream, clock, p2pProvider, isFirstChunk)
	// 	if errors.Is(err, io.EOF) {
	// 		break
	// 	}
	// 	if err != nil {
	// 		return nil, err
	// 	}

	// 	if err := process(blk); err != nil {
	// 		return nil, err
	// 	}
	// }

	return execution_proofs, nil
}

// executionProofsByRootRPCHandler looks up the request blocks from the database from the given block roots.
func (s *Service) executionProofsByRootRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	_, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	// log := log.WithField("handler", "execution_proof_by_root")

	rawMsg, ok := msg.(*p2ptypes.ExecutionProofByRootsReq)
	if !ok {
		return errors.New("message is not type ExecutionProofByRootsReq")
	}
	blockRoots := *rawMsg
	if err := s.rateLimiter.validateRequest(stream, uint64(len(blockRoots))); err != nil {
		return err
	}
	if len(blockRoots) == 0 {
		// Add to rate limiter in the event no
		// roots are requested.
		s.rateLimiter.add(stream, 1)
		s.writeErrorResponseToStream(responseCodeInvalidRequest, "no block roots provided in request", stream)
		return errors.New("no block roots provided")
	}

	s.rateLimiter.add(stream, int64(len(blockRoots)))

	// for _, root := range blockRoots {
	// 	blk, err := s.cfg.beaconDB.Block(ctx, root)
	// 	if err != nil {
	// 		log.WithError(err).Debug("Could not fetch block")
	// 		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
	// 		return err
	// 	}
	// 	if err := blocks.BeaconBlockIsNil(blk); err != nil {
	// 		continue
	// 	}

	// 	if blk.Block().IsBlinded() {
	// 		blk, err = s.cfg.executionReconstructor.ReconstructFullBlock(ctx, blk)
	// 		if err != nil {
	// 			if errors.Is(err, execution.ErrEmptyBlockHash) {
	// 				log.WithError(err).Warn("Could not reconstruct block from header with syncing execution client. Waiting to complete syncing")
	// 			} else {
	// 				log.WithError(err).Error("Could not get reconstruct full block from blinded body")
	// 			}
	// 			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
	// 			return err
	// 		}
	// 	}

	// 	if err := s.chunkBlockWriter(stream, blk); err != nil {
	// 		return err
	// 	}
	// }

	// closeStream(stream, log)
	return nil
}
