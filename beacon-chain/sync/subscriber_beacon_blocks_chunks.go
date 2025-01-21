package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	core_chunks "github.com/OffchainLabs/prysm/v6/beacon-chain/core/chunks"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync/rlnc"
	"github.com/OffchainLabs/prysm/v6/consensus-types/chunks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func (s *Service) beaconBlockChunkSubscriber(ctx context.Context, msg proto.Message) error {
	chunk, err := chunks.NewBlockChunk(msg)
	if err != nil {
		return err
	}
	if chunk.IsNil() {
		return chunks.ErrNilObject
	}
	_, err = s.blockChunkCache.GetBlockData(chunk.Slot(), chunk.ProposerIndex())
	if err != nil {
		//nolint:nilerr // Only error is ErrNoData when the node is not full.
		return nil
	}
	// TODO, implement block reconstruction from the byte slice
	return nil
}

func (s *Service) validateBeaconBlockChunkPubSub(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}
	ctx, span := trace.StartSpan(ctx, "sync.validateBeaconBlockChunkPubSub")
	defer span.End()

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, errors.Wrap(err, "Could not decode message")
	}

	// It's fine to use the same lock for both block and chunk validation
	s.validateBlockLock.Lock()
	defer s.validateBlockLock.Unlock()

	chunk, ok := m.(interfaces.ReadOnlyBeaconBlockChunk)
	if !ok {
		return pubsub.ValidationReject, errors.New("msg is not ReadOnlyBeaconBlockChunk")
	}

	if chunk.IsNil() {
		return pubsub.ValidationReject, errors.New("chunk is nil")
	}

	// Check if parent is a bad block and then reject the chunk.
	if s.hasBadBlock(chunk.ParentRoot()) {
		err := fmt.Errorf("received chunk that has an invalid parent %#x", chunk.ParentRoot())
		log.WithError(err).Debug("Received block with an invalid parent")
		return pubsub.ValidationReject, err
	}

	// Be lenient in handling early blocks. Instead of discarding blocks arriving later than
	// MAXIMUM_GOSSIP_CLOCK_DISPARITY in future, we tolerate blocks arriving at max two slots
	// earlier (SECONDS_PER_SLOT * 2 seconds). Queue such blocks and process them at the right slot.
	genesisTime := uint64(s.cfg.clock.GenesisTime().Unix())
	if err := slots.VerifyTime(genesisTime, chunk.Slot(), earlyBlockProcessingTolerance); err != nil {
		log.WithError(err).Debug("Ignored chunk: could not verify slot time")
		return pubsub.ValidationIgnore, nil
	}

	cp := s.cfg.chain.FinalizedCheckpt()
	startSlot, err := slots.EpochStart(cp.Epoch)
	if err != nil {
		log.WithError(err).Debug("Ignored block: could not calculate epoch start slot")
		return pubsub.ValidationIgnore, nil
	}
	if startSlot >= chunk.Slot() {
		err := fmt.Errorf("finalized slot %d greater or equal to block slot %d", startSlot, chunk.Slot())
		log.Debug(err)
		return pubsub.ValidationIgnore, err
	}

	if !s.cfg.chain.HasBlock(ctx, chunk.ParentRoot()) {
		// TODO: implement pending chunk storage
		return pubsub.ValidationIgnore, err
	}

	// We ignore messages instead of accepting them to avoid rebroadcasting by gossipsub.
	err = s.validateBeaconBlockChunk(ctx, chunk)
	if errors.Is(err, rlnc.ErrLinearlyDependentMessage) {
		log.Debug("ignoring linearly dependent message")
		return pubsub.ValidationIgnore, nil
	} else if err != nil {
		// TODO: cook up a slashing object if the error is ErrIncorrectCommitments
		log.WithError(err).Debug("Could not validate beacon block chunk")
		return pubsub.ValidationReject, err
	}
	logFields := logrus.Fields{
		"chunkSlot":     chunk.Slot(),
		"proposerIndex": chunk.ProposerIndex(),
		"parentRoot":    chunk.ParentRoot(),
	}
	log.WithFields(logFields).Debug("Received block chunk")

	// If the block can be recovered, send it to the blockchain package
	go s.reconstructBlockFromChunk(ctx, chunk)
	return pubsub.ValidationIgnore, nil
}

func (s *Service) validateBeaconBlockChunk(ctx context.Context, chunk interfaces.ReadOnlyBeaconBlockChunk) error {
	if !s.cfg.chain.InForkchoice(chunk.ParentRoot()) {
		return blockchain.ErrNotDescendantOfFinalized
	}
	err := s.blockChunkCache.AddChunk(chunk)
	if err == nil {
		return nil
	}
	if errors.Is(err, rlnc.ErrSignatureNotVerified) {
		parentState, err := s.cfg.stateGen.StateByRoot(ctx, chunk.ParentRoot())
		if err != nil {
			s.blockChunkCache.RemoveNode(chunk) // Node is guaranteed to have a single chunk
			return err
		}
		if err := core_chunks.VerifyChunkSignatureUsingCurrentFork(parentState, chunk); err != nil {
			s.blockChunkCache.RemoveNode(chunk) // Node is guaranteed to have a single chunk
			return err
		}
		return nil
	}
	return err
}

// startChunkPruner starts a goroutine that prunes the block chunk cache every epoch.
func (s *Service) startChunkPruner() {
	cp := s.cfg.chain.FinalizedCheckpt()
	fSlot, err := slots.EpochStart(cp.Epoch)
	if err != nil {
		log.WithError(err).Debug("could not prune the chunk cache: could not calculate epoch start slot")
	} else {
		s.blockChunkCache.Prune(fSlot)
	}
}

func (s *Service) reconstructBlockFromChunk(ctx context.Context, chunk interfaces.ReadOnlyBeaconBlockChunk) {
	data, err := s.blockChunkCache.GetBlockData(chunk.Slot(), chunk.ProposerIndex())
	if err != nil {
		return
	}

	msg := p2p.GossipTopicMappings(p2p.BlockSubnetTopicFormat, slots.ToEpoch(chunk.Slot()))
	e := &encoder.SszNetworkEncoder{}

	if err := e.DecodeGossip(data, msg); err != nil {
		logrus.WithError(err).Error("Could not decode block data")
		return
	}
	if err := s.beaconBlockSubscriber(ctx, msg); err != nil {
		logrus.WithError(err).Error("Could not handle p2p pubsub")
	}
}
