package sync

import (
	"context"
	"fmt"
	"strings"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	coreBlocks "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

func (s *Service) validateDataColumn(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	receivedTime := prysmTime.Now()

	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}
	if msg.Topic == nil {
		return pubsub.ValidationReject, errInvalidTopic
	}
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		log.WithError(err).Error("Failed to decode message")
		return pubsub.ValidationReject, err
	}

	ds, ok := m.(*eth.DataColumnSidecar)
	if !ok {
		log.WithField("message", m).Error("Message is not of type *eth.DataColumnSidecar")
		return pubsub.ValidationReject, errWrongMessage
	}
	if ds.ColumnIndex >= params.BeaconConfig().NumberOfColumns {
		return pubsub.ValidationReject, errors.Errorf("invalid column index provided, got %d", ds.ColumnIndex)
	}
	want := fmt.Sprintf("data_column_sidecar_%d", computeSubnetForColumnSidecar(ds.ColumnIndex))
	if !strings.Contains(*msg.Topic, want) {
		log.Debug("Column Sidecar index does not match topic")
		return pubsub.ValidationReject, fmt.Errorf("wrong topic name: %s", *msg.Topic)
	}
	if err := slots.VerifyTime(uint64(s.cfg.clock.GenesisTime().Unix()), ds.SignedBlockHeader.Header.Slot, params.BeaconConfig().MaximumGossipClockDisparityDuration()); err != nil {
		log.WithError(err).Debug("Ignored sidecar: could not verify slot time")
		return pubsub.ValidationIgnore, nil
	}
	cp := s.cfg.chain.FinalizedCheckpt()
	startSlot, err := slots.EpochStart(cp.Epoch)
	if err != nil {
		log.WithError(err).Debug("Ignored column sidecar: could not calculate epoch start slot")
		return pubsub.ValidationIgnore, nil
	}
	if startSlot >= ds.SignedBlockHeader.Header.Slot {
		err := fmt.Errorf("finalized slot %d greater or equal to block slot %d", startSlot, ds.SignedBlockHeader.Header.Slot)
		log.Debug(err)
		return pubsub.ValidationIgnore, err
	}
	// Handle sidecar when the parent is unknown.
	if !s.cfg.chain.HasBlock(ctx, [32]byte(ds.SignedBlockHeader.Header.ParentRoot)) {
		err := errors.Errorf("unknown parent for data column sidecar with slot %d and parent root %#x", ds.SignedBlockHeader.Header.Slot, ds.SignedBlockHeader.Header.ParentRoot)
		log.WithError(err).Debug("Could not identify parent for data column sidecar")
		return pubsub.ValidationIgnore, err
	}
	if s.hasBadBlock([32]byte(ds.SignedBlockHeader.Header.ParentRoot)) {
		bRoot, err := ds.SignedBlockHeader.Header.HashTreeRoot()
		if err != nil {
			return pubsub.ValidationIgnore, err
		}
		s.setBadBlock(ctx, bRoot)
		return pubsub.ValidationReject, errors.Errorf("column sidecar with bad parent provided")
	}
	parentSlot, err := s.cfg.chain.RecentBlockSlot([32]byte(ds.SignedBlockHeader.Header.ParentRoot))
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if ds.SignedBlockHeader.Header.Slot <= parentSlot {
		return pubsub.ValidationReject, errors.Errorf("invalid column sidecar slot: %d", ds.SignedBlockHeader.Header.Slot)
	}
	if !s.cfg.chain.InForkchoice([32]byte(ds.SignedBlockHeader.Header.ParentRoot)) {
		return pubsub.ValidationReject, blockchain.ErrNotDescendantOfFinalized
	}
	// TODO Verify KZG inclusion proof of data column sidecar

	// TODO Verify KZG proofs of column sidecar

	parentState, err := s.cfg.stateGen.StateByRoot(ctx, [32]byte(ds.SignedBlockHeader.Header.ParentRoot))
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	if err := coreBlocks.VerifyBlockHeaderSignatureUsingCurrentFork(parentState, ds.SignedBlockHeader); err != nil {
		return pubsub.ValidationReject, err
	}
	// In the event the block is more than an epoch ahead from its
	// parent state, we have to advance the state forward.
	parentRoot := ds.SignedBlockHeader.Header.ParentRoot
	parentState, err = transition.ProcessSlotsUsingNextSlotCache(ctx, parentState, parentRoot, ds.SignedBlockHeader.Header.Slot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	idx, err := helpers.BeaconProposerIndex(ctx, parentState)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if ds.SignedBlockHeader.Header.ProposerIndex != idx {
		return pubsub.ValidationReject, errors.New("incorrect proposer index")
	}

	startTime, err := slots.ToTime(uint64(s.cfg.chain.GenesisTime().Unix()), ds.SignedBlockHeader.Header.Slot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	sinceSlotStartTime := receivedTime.Sub(startTime)
	validationTime := s.cfg.clock.Now().Sub(receivedTime)

	log.WithFields(logrus.Fields{
		"sinceSlotStartTime": sinceSlotStartTime,
		"validationTime":     validationTime,
	}).Debug("Received data column sidecar")

	// TODO: Transform this whole function so it looks like to the `validateBlob`
	// with the tiny verifiers inside.
	roDataColumn, err := blocks.NewRODataColumn(ds)
	if err != nil {
		return pubsub.ValidationReject, errors.Wrap(err, "new RO data columns")
	}

	verifiedRODataColumn := blocks.NewVerifiedRODataColumn(roDataColumn)

	msg.ValidatorData = verifiedRODataColumn
	return pubsub.ValidationAccept, nil
}

// Sets the data column with the same slot, proposer index, and data column index as seen.
func (s *Service) setSeenDataColumnIndex(slot primitives.Slot, proposerIndex primitives.ValidatorIndex, index uint64) {
	s.seenDataColumnLock.Lock()
	defer s.seenDataColumnLock.Unlock()

	b := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(proposerIndex))...)
	b = append(b, bytesutil.Bytes32(index)...)
	s.seenDataColumnCache.Add(string(b), true)
}

func computeSubnetForColumnSidecar(colIdx uint64) uint64 {
	return colIdx % params.BeaconConfig().DataColumnSidecarSubnetCount
}
