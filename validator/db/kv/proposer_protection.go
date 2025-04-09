package kv

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
	bolt "go.etcd.io/bbolt"
)

// ProposedPublicKeys retrieves all public keys in our proposals history bucket.
// Warning: A public key in this bucket does not necessarily mean it has signed a block.
func (s *Store) ProposedPublicKeys(ctx context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
	_, span := trace.StartSpan(ctx, "Validator.ProposedPublicKeys")
	defer span.End()
	var err error
	proposedPublicKeys := make([][fieldparams.BLSPubkeyLength]byte, 0)
	err = s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(historicProposalsBucket)
		return bucket.ForEach(func(key []byte, _ []byte) error {
			var pubKeyBytes [fieldparams.BLSPubkeyLength]byte
			copy(pubKeyBytes[:], key)
			proposedPublicKeys = append(proposedPublicKeys, pubKeyBytes)
			return nil
		})
	})
	return proposedPublicKeys, err
}

// ProposalHistoryForSlot accepts a validator public key and returns the corresponding signing root as well
// as a boolean that tells us if we have a proposal history stored at the slot and a boolean that tells us if we have
// a signed root at the slot.
func (s *Store) ProposalHistoryForSlot(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte, slot primitives.Slot) ([32]byte, bool, bool, error) {
	_, span := trace.StartSpan(ctx, "Validator.ProposalHistoryForSlot")
	defer span.End()

	var (
		err                               error
		proposalExists, signingRootExists bool
		signingRoot                       [32]byte
	)

	err = s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(historicProposalsBucket)
		valBucket := bucket.Bucket(publicKey[:])
		if valBucket == nil {
			return nil
		}

		signingRootBytes := valBucket.Get(bytesutil.SlotToBytesBigEndian(slot))
		if signingRootBytes == nil {
			return nil
		}

		// If we are at this point, we are sure we have a proposal history for the slot.
		proposalExists = true
		if len(signingRootBytes) == 0 {
			return nil
		}

		// If we are at this point, we are sure we have a signing root for the slot.
		signingRootExists = true
		copy(signingRoot[:], signingRootBytes)
		return nil
	})
	return signingRoot, proposalExists, signingRootExists, err
}

// ProposalHistoryForPubKey returns the entire proposal history for a given public key.
func (s *Store) ProposalHistoryForPubKey(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) ([]*common.Proposal, error) {
	_, span := trace.StartSpan(ctx, "Validator.ProposalHistoryForPubKey")
	defer span.End()

	proposals := make([]*common.Proposal, 0)
	err := s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(historicProposalsBucket)
		valBucket := bucket.Bucket(publicKey[:])
		if valBucket == nil {
			return nil
		}
		return valBucket.ForEach(func(slotKey, signingRootBytes []byte) error {
			slot := bytesutil.BytesToSlotBigEndian(slotKey)
			sr := make([]byte, fieldparams.RootLength)
			copy(sr, signingRootBytes)
			proposals = append(proposals, &common.Proposal{
				Slot:        slot,
				SigningRoot: sr,
			})
			return nil
		})
	})
	return proposals, err
}

// SaveProposalHistoryForSlot saves the proposal history for the requested validator public key.
// We also check if the incoming proposal slot is lower than the lowest signed proposal slot
// for the validator and override its value on disk.
func (s *Store) SaveProposalHistoryForSlot(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, slot primitives.Slot, signingRoot []byte) error {
	_, span := trace.StartSpan(ctx, "Validator.SaveProposalHistoryForEpoch")
	defer span.End()

	err := s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(historicProposalsBucket)
		valBucket, err := bucket.CreateBucketIfNotExists(pubKey[:])
		if err != nil {
			return fmt.Errorf("could not create bucket for public key %#x", pubKey)
		}

		// If the incoming slot is lower than the lowest signed proposal slot, override.
		lowestSignedBkt := tx.Bucket(lowestSignedProposalsBucket)
		lowestSignedProposalBytes := lowestSignedBkt.Get(pubKey[:])
		var lowestSignedProposalSlot primitives.Slot
		if len(lowestSignedProposalBytes) >= 8 {
			lowestSignedProposalSlot = bytesutil.BytesToSlotBigEndian(lowestSignedProposalBytes)
		}
		if len(lowestSignedProposalBytes) == 0 || slot < lowestSignedProposalSlot {
			if err := lowestSignedBkt.Put(pubKey[:], bytesutil.SlotToBytesBigEndian(slot)); err != nil {
				return err
			}
		}

		// If the incoming slot is higher than the highest signed proposal slot, override.
		highestSignedBkt := tx.Bucket(highestSignedProposalsBucket)
		highestSignedProposalBytes := highestSignedBkt.Get(pubKey[:])
		var highestSignedProposalSlot primitives.Slot
		if len(highestSignedProposalBytes) >= 8 {
			highestSignedProposalSlot = bytesutil.BytesToSlotBigEndian(highestSignedProposalBytes)
		}
		if len(highestSignedProposalBytes) == 0 || slot > highestSignedProposalSlot {
			if err := highestSignedBkt.Put(pubKey[:], bytesutil.SlotToBytesBigEndian(slot)); err != nil {
				return err
			}
		}

		if err := valBucket.Put(bytesutil.SlotToBytesBigEndian(slot), signingRoot); err != nil {
			return err
		}
		return pruneProposalHistoryBySlot(valBucket, slot)
	})
	return err
}

// LowestSignedProposal returns the lowest signed proposal slot for a validator public key.
// If no data exists, a boolean of value false is returned.
func (s *Store) LowestSignedProposal(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) (primitives.Slot, bool, error) {
	_, span := trace.StartSpan(ctx, "Validator.LowestSignedProposal")
	defer span.End()

	var err error
	var lowestSignedProposalSlot primitives.Slot
	var exists bool
	err = s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(lowestSignedProposalsBucket)
		lowestSignedProposalBytes := bucket.Get(publicKey[:])
		// 8 because bytesutil.BytesToUint64BigEndian will return 0 if input is less than 8 bytes.
		if len(lowestSignedProposalBytes) < 8 {
			return nil
		}
		exists = true
		lowestSignedProposalSlot = bytesutil.BytesToSlotBigEndian(lowestSignedProposalBytes)
		return nil
	})
	return lowestSignedProposalSlot, exists, err
}

// HighestSignedProposal returns the highest signed proposal slot for a validator public key.
// If no data exists, a boolean of value false is returned.
func (s *Store) HighestSignedProposal(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) (primitives.Slot, bool, error) {
	_, span := trace.StartSpan(ctx, "Validator.HighestSignedProposal")
	defer span.End()

	var err error
	var highestSignedProposalSlot primitives.Slot
	var exists bool
	err = s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(highestSignedProposalsBucket)
		highestSignedProposalBytes := bucket.Get(publicKey[:])
		// 8 because bytesutil.BytesToUint64BigEndian will return 0 if input is less than 8 bytes.
		if len(highestSignedProposalBytes) < 8 {
			return nil
		}
		exists = true
		highestSignedProposalSlot = bytesutil.BytesToSlotBigEndian(highestSignedProposalBytes)
		return nil
	})
	return highestSignedProposalSlot, exists, err
}

// SlashableProposalCheck checks if a block proposal is slashable by comparing it with the
// block proposals history for the given public key in our complete slashing protection database defined by EIP-3076.
// If it is not, we then update the history.
func (s *Store) SlashableProposalCheck(
	ctx context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	signedBlock interfaces.ReadOnlySignedBeaconBlock,
	signingRoot [fieldparams.RootLength]byte,
	emitAccountMetrics bool,
	validatorProposeFailVec *prometheus.CounterVec,
) error {
	fmtKey := fmt.Sprintf("%#x", pubKey[:])

	blk := signedBlock.Block()
	prevSigningRoot, proposalAtSlotExists, prevSigningRootExists, err := s.ProposalHistoryForSlot(ctx, pubKey, blk.Slot())
	if err != nil {
		if emitAccountMetrics {
			validatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return errors.Wrap(err, "failed to get proposal history")
	}

	lowestSignedProposalSlot, lowestProposalExists, err := s.LowestSignedProposal(ctx, pubKey)
	if err != nil {
		return err
	}

	// Based on EIP-3076 - Condition 2
	// -------------------------------
	if lowestProposalExists {
		// If the block slot is (strictly) less than the lowest signed proposal slot in the DB, we consider it slashable.
		if blk.Slot() < lowestSignedProposalSlot {
			return fmt.Errorf(
				"could not sign block with slot < lowest signed slot in db, block slot: %d < lowest signed slot: %d",
				blk.Slot(),
				lowestSignedProposalSlot,
			)
		}

		// If the block slot is equal to the lowest signed proposal slot and
		// - condition1: there is no signed proposal in the DB for this slot, or
		// - condition2: there is  a signed proposal in the DB for this slot, but with no associated signing root, or
		// - condition3: there is  a signed proposal in the DB for this slot, but the signing root differs,
		// ==> we consider it slashable.
		condition1 := !proposalAtSlotExists
		condition2 := proposalAtSlotExists && !prevSigningRootExists
		condition3 := proposalAtSlotExists && prevSigningRootExists && prevSigningRoot != signingRoot
		if blk.Slot() == lowestSignedProposalSlot && (condition1 || condition2 || condition3) {
			return fmt.Errorf(
				"could not sign block with slot == lowest signed slot in db if it is not a repeat signing, block slot: %d == slowest signed slot: %d",
				blk.Slot(),
				lowestSignedProposalSlot,
			)
		}
	}

	// Based on EIP-3076 - Condition 1
	// -------------------------------
	// If there is a signed proposal in the DB for this slot and
	// - there is no associated signing root, or
	// - the signing root differs,
	// ==> we consider it slashable.
	if proposalAtSlotExists && (!prevSigningRootExists || prevSigningRoot != signingRoot) {
		if emitAccountMetrics {
			validatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return errors.New(common.FailedBlockSignLocalErr)
	}

	// Save the proposal for this slot.
	if err := s.SaveProposalHistoryForSlot(ctx, pubKey, blk.Slot(), signingRoot[:]); err != nil {
		if emitAccountMetrics {
			validatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return errors.Wrap(err, "failed to save updated proposal history")
	}

	return nil
}

func pruneProposalHistoryBySlot(valBucket *bolt.Bucket, newestSlot primitives.Slot) error {
	c := valBucket.Cursor()
	for k, _ := c.First(); k != nil; k, _ = c.First() {
		slot := bytesutil.BytesToSlotBigEndian(k)
		epoch := slots.ToEpoch(slot)
		newestEpoch := slots.ToEpoch(newestSlot)
		// Only delete epochs that are older than the weak subjectivity period.
		if epoch+params.BeaconConfig().WeakSubjectivityPeriod <= newestEpoch {
			if err := c.Delete(); err != nil {
				return errors.Wrapf(err, "could not prune epoch %d in proposal history", epoch)
			}
		} else {
			// If starting from the oldest, we don't find anything prunable, stop pruning.
			break
		}
	}
	return nil
}
