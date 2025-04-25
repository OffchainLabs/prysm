package filesystem

import (
	"context"
	"strings"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/validator/db/common"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// HighestSignedProposal is implemented only to satisfy the interface.
func (*Store) HighestSignedProposal(_ context.Context, _ [fieldparams.BLSPubkeyLength]byte) (primitives.Slot, bool, error) {
	return 0, false, errors.New("not implemented")
}

// LowestSignedProposal is implemented only to satisfy the interface.
func (*Store) LowestSignedProposal(_ context.Context, _ [fieldparams.BLSPubkeyLength]byte) (primitives.Slot, bool, error) {
	return 0, false, errors.New("not implemented")
}

// ProposalHistoryForPubKey returns the proposal history for a given public key.
func (s *Store) ProposalHistoryForPubKey(_ context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) ([]*common.Proposal, error) {
	// Get validator slashing protection.
	validatorSlashingProtection, err := s.validatorSlashingProtection(publicKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not get validator slashing protection")
	}

	// If there is no validator slashing protection or proposed block, return an empty slice.
	if validatorSlashingProtection == nil || validatorSlashingProtection.LatestSignedBlockSlot == nil {
		return []*common.Proposal{}, nil
	}

	// Return the (unique) proposal history.
	return []*common.Proposal{
		{
			Slot: primitives.Slot(*validatorSlashingProtection.LatestSignedBlockSlot),
		},
	}, nil
}

// ProposalHistoryForSlot is implemented only to satisfy the interface.
func (*Store) ProposalHistoryForSlot(_ context.Context, _ [fieldparams.BLSPubkeyLength]byte, _ primitives.Slot) ([fieldparams.RootLength]byte, bool, bool, error) {
	return [32]byte{}, false, false, errors.New("not implemented")
}

// SaveProposalHistoryForSlot checks if the incoming proposal is valid regarding EIP-3076 minimal slashing protection.
// If so, it updates the database with the incoming slot, and returns nil.
// If not, it does not modify the database and return an error.
func (s *Store) SaveProposalHistoryForSlot(
	_ context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	slot primitives.Slot,
	_ []byte,
) error {
	// Get validator slashing protection.
	validatorSlashingProtection, err := s.validatorSlashingProtection(pubKey)
	if err != nil {
		return errors.Wrap(err, "could not get validator slashing protection")
	}

	// Convert the slot to uint64.
	slotUInt64 := uint64(slot)

	if validatorSlashingProtection == nil {
		// If there is no validator slashing protection, create one
		validatorSlashingProtection = &ValidatorSlashingProtection{
			LatestSignedBlockSlot: &slotUInt64,
		}

		// Save the validator slashing protection.
		if err := s.saveValidatorSlashingProtection(pubKey, validatorSlashingProtection); err != nil {
			return errors.Wrap(err, "could not save validator slashing protection")
		}

		return nil
	}

	if validatorSlashingProtection.LatestSignedBlockSlot == nil {
		// If there is no latest signed block slot, update it.
		validatorSlashingProtection.LatestSignedBlockSlot = &slotUInt64

		// Save the validator slashing protection.
		if err := s.saveValidatorSlashingProtection(pubKey, validatorSlashingProtection); err != nil {
			return errors.Wrap(err, "could not save validator slashing protection")
		}

		return nil
	}

	// Based on EIP-3076 (minimal database), validator should refuse to sign any proposal
	// with slot less than or equal to the latest signed block slot in the DB.
	if slotUInt64 <= *validatorSlashingProtection.LatestSignedBlockSlot {
		return errors.Errorf(
			"could not sign proposal with slot lower than or equal to recorded slot, %d <= %d",
			slot,
			*validatorSlashingProtection.LatestSignedBlockSlot,
		)
	}

	// Update the latest signed block slot.
	validatorSlashingProtection.LatestSignedBlockSlot = &slotUInt64

	// Save the validator slashing protection.
	if err := s.saveValidatorSlashingProtection(pubKey, validatorSlashingProtection); err != nil {
		return errors.Wrap(err, "could not save validator slashing protection")
	}

	return nil
}

// ProposedPublicKeys returns the list of public keys we have in the database.
// To be consistent with the complete, BoltDB implementation, pubkeys returned by
// this function do not necessarily have proposed a block.
func (s *Store) ProposedPublicKeys(_ context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
	return s.publicKeys()
}

// SlashableProposalCheck checks if a block proposal is slashable by comparing it with the
// block proposals history for the given public key in our minimal slashing protection database defined by EIP-3076.
// If it is not, it update the database.
func (s *Store) SlashableProposalCheck(
	ctx context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	signedBlock interfaces.ReadOnlySignedBeaconBlock,
	signingRoot [fieldparams.RootLength]byte,
	emitAccountMetrics bool,
	validatorProposeFailVec *prometheus.CounterVec,
) error {
	// Check if the proposal is potentially slashable regarding EIP-3076 minimal conditions.
	// If not, save the new proposal into the database.
	if err := s.SaveProposalHistoryForSlot(ctx, pubKey, signedBlock.Block().Slot(), signingRoot[:]); err != nil {
		if strings.Contains(err.Error(), "could not sign proposal") {
			return errors.Wrapf(err, common.FailedBlockSignLocalErr)
		}

		return errors.Wrap(err, "failed to save updated proposal history")
	}

	return nil
}
