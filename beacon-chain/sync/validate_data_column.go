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
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

// https://github.com/ethereum/consensus-specs/blob/dev/specs/_features/eip7594/p2p-interface.md#the-gossip-domain-gossipsub
func (s *Service) validateDataColumn(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	receivedTime := prysmTime.Now()

	// Always accept messages our own messages.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Ignore messages during initial sync.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	// Ignore message with a nil topic.
	if msg.Topic == nil {
		return pubsub.ValidationReject, errInvalidTopic
	}

	// Decode the message.
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		log.WithError(err).Error("Failed to decode message")
		return pubsub.ValidationReject, err
	}

	// Ignore messages that are not of the expected type.
	ds, ok := m.(*eth.DataColumnSidecar)
	if !ok {
		log.WithField("message", m).Error("Message is not of type *eth.DataColumnSidecar")
		return pubsub.ValidationReject, errWrongMessage
	}

	// [REJECT] The sidecar's index is consistent with NUMBER_OF_COLUMNS -- i.e. sidecar.index < NUMBER_OF_COLUMNS.
	if ds.ColumnIndex >= params.BeaconConfig().NumberOfColumns {
		return pubsub.ValidationReject, errors.Errorf("invalid column index provided, got %d", ds.ColumnIndex)
	}

	// [REJECT] The sidecar is for the correct subnet -- i.e. compute_subnet_for_data_column_sidecar(sidecar.index) == subnet_id.
	want := fmt.Sprintf("data_column_sidecar_%d", computeSubnetForColumnSidecar(ds.ColumnIndex))
	if !strings.Contains(*msg.Topic, want) {
		log.Debug("Column Sidecar index does not match topic")
		return pubsub.ValidationReject, fmt.Errorf("wrong topic name: %s", *msg.Topic)
	}

	// [IGNORE] The sidecar is not from a future slot (with a MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance) -- i.e. validate that block_header.slot <= current_slot (a client MAY queue future sidecars for processing at the appropriate slot).
	if err := slots.VerifyTime(uint64(s.cfg.clock.GenesisTime().Unix()), ds.SignedBlockHeader.Header.Slot, params.BeaconConfig().MaximumGossipClockDisparityDuration()); err != nil {
		log.WithError(err).Debug("Ignored sidecar: could not verify slot time")
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] The sidecar is from a slot greater than the latest finalized slot -- i.e. validate that block_header.slot > compute_start_slot_at_epoch(state.finalized_checkpoint.epoch)
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

	// [IGNORE] The sidecar's block's parent (defined by block_header.parent_root) has been seen (via both gossip and non-gossip sources) (a client MAY queue sidecars for processing once the parent block is retrieved).
	if !s.cfg.chain.HasBlock(ctx, [32]byte(ds.SignedBlockHeader.Header.ParentRoot)) {
		err := errors.Errorf("unknown parent for data column sidecar with slot %d and parent root %#x", ds.SignedBlockHeader.Header.Slot, ds.SignedBlockHeader.Header.ParentRoot)
		log.WithError(err).Debug("Could not identify parent for data column sidecar")
		return pubsub.ValidationIgnore, err
	}

	// [REJECT] The sidecar's block's parent (defined by block_header.parent_root) passes validation.
	if s.hasBadBlock([32]byte(ds.SignedBlockHeader.Header.ParentRoot)) {
		bRoot, err := ds.SignedBlockHeader.Header.HashTreeRoot()
		if err != nil {
			return pubsub.ValidationIgnore, err
		}

		// If parent is bad, we set the block as bad.
		s.setBadBlock(ctx, bRoot)
		return pubsub.ValidationReject, errors.Errorf("column sidecar with bad parent provided")
	}

	// [REJECT] The sidecar is from a higher slot than the sidecar's block's parent (defined by block_header.parent_root).
	parentSlot, err := s.cfg.chain.RecentBlockSlot([32]byte(ds.SignedBlockHeader.Header.ParentRoot))
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	if ds.SignedBlockHeader.Header.Slot <= parentSlot {
		return pubsub.ValidationReject, errors.Errorf("invalid column sidecar slot: %d", ds.SignedBlockHeader.Header.Slot)
	}

	// [REJECT] The current finalized_checkpoint is an ancestor of the sidecar's block -- i.e. get_checkpoint_block(store, block_header.parent_root, store.finalized_checkpoint.epoch) == store.finalized_checkpoint.root.
	if !s.cfg.chain.InForkchoice([32]byte(ds.SignedBlockHeader.Header.ParentRoot)) {
		return pubsub.ValidationReject, blockchain.ErrNotDescendantOfFinalized
	}

	// [REJECT] The sidecar's kzg_commitments field inclusion proof is valid as verified by verify_data_column_sidecar_inclusion_proof(sidecar).
	if err := blocks.VerifyKZGInclusionProofColumn(ds); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar's column data is valid as verified by verify_data_column_sidecar_kzg_proofs(sidecar).
	verified, err := peerdas.VerifyDataColumnSidecarKZGProofs(ds)
	if err != nil {
		return pubsub.ValidationReject, err
	}

	if !verified {
		return pubsub.ValidationReject, errors.New("failed to verify kzg proof of column")
	}

	// [REJECT] The proposer signature of sidecar.signed_block_header, is valid with respect to the block_header.proposer_index pubkey.
	parentState, err := s.cfg.stateGen.StateByRoot(ctx, [32]byte(ds.SignedBlockHeader.Header.ParentRoot))
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	if err := coreBlocks.VerifyBlockHeaderSignatureUsingCurrentFork(parentState, ds.SignedBlockHeader); err != nil {
		return pubsub.ValidationReject, err
	}
	roDataColumn, err := blocks.NewRODataColumn(ds)
	if err != nil {
		return pubsub.ValidationReject, errors.Wrap(err, "new RO data columns")
	}

	if err := s.newColumnProposerVerifier(ctx, roDataColumn); err != nil {
		return pubsub.ValidationReject, errors.Wrap(err, "could not verify proposer")
	}

	// Get the time at slot start.
	startTime, err := slots.ToTime(uint64(s.cfg.chain.GenesisTime().Unix()), ds.SignedBlockHeader.Header.Slot)

	// Add specific debug log.
	if err == nil {
		log.WithFields(logrus.Fields{
			"sinceSlotStartTime": receivedTime.Sub(startTime),
			"validationTime":     s.cfg.clock.Now().Sub(receivedTime),
			"columnIndex":        ds.ColumnIndex,
		}).Debug("Received data column sidecar")
	} else {
		log.WithError(err).Error("Failed to calculate slot time")
	}

	// TODO: Transform this whole function so it looks like to the `validateBlob`
	// with the tiny verifiers inside.
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
