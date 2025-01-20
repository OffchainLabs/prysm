package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	core_chunks "github.com/OffchainLabs/prysm/v6/beacon-chain/core/chunks"
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
	// TODO: verify if we have the full block, decode and send it to the blockchain package.
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

	err = s.validateBeaconBlockChunk(ctx, chunk)
	if err != nil {
		log.WithError(err).Debug("Could not validate beacon block chunk")
		return pubsub.ValidationReject, err
	}

	logFields := logrus.Fields{
		"chunkSlot":     chunk.Slot(),
		"proposerIndex": chunk.ProposerIndex(),
		"parentRoot":    chunk.ParentRoot(),
	}
	log.WithFields(logFields).Debug("Received block chunk")
	return pubsub.ValidationAccept, nil
}

func (s *Service) validateBeaconBlockChunk(ctx context.Context, chunk interfaces.ReadOnlyBeaconBlockChunk) error {
	if !s.cfg.chain.InForkchoice(chunk.ParentRoot()) {
		return blockchain.ErrNotDescendantOfFinalized
	}

	parentState, err := s.cfg.stateGen.StateByRoot(ctx, chunk.ParentRoot())
	if err != nil {
		return err
	}

	if err := core_chunks.VerifyChunkSignatureUsingCurrentFork(parentState, chunk); err != nil {
		return err
	}
	return nil
}
