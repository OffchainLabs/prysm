package filesystem

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
)

const failedAttLocalProtectionErr = "attempted to make slashable attestation, rejected by local slashing protection"

// EIPImportBlacklistedPublicKeys is implemented only to satisfy the interface.
func (*Store) EIPImportBlacklistedPublicKeys(_ context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
	return [][fieldparams.BLSPubkeyLength]byte{}, nil
}

// SaveEIPImportBlacklistedPublicKeys is implemented only to satisfy the interface.
func (*Store) SaveEIPImportBlacklistedPublicKeys(_ context.Context, _ [][fieldparams.BLSPubkeyLength]byte) error {
	return nil
}

// SigningRootAtTargetEpoch is implemented only to satisfy the interface.
func (*Store) SigningRootAtTargetEpoch(_ context.Context, _ [fieldparams.BLSPubkeyLength]byte, _ primitives.Epoch) ([]byte, error) {
	panic("not implemented")
}

// LowestSignedTargetEpoch returns the lowest signed target epoch for a public key, a boolean indicating if it exists and an error.
func (s *Store) LowestSignedTargetEpoch(_ context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) (primitives.Epoch, bool, error) {
	// Get validator slashing protection.
	validatorSlashingProtection, err := s.validatorSlashingProtection(pubKey)
	if err != nil {
		return 0, false, errors.Wrap(err, "could not get validator slashing protection")
	}

	// If there is no validator slashing protection, return early.
	if validatorSlashingProtection == nil || validatorSlashingProtection.LastSignedAttestationTargetEpoch == nil {
		return 0, false, nil
	}

	// Return the lowest (and unique) signed target epoch.
	return primitives.Epoch(*validatorSlashingProtection.LastSignedAttestationTargetEpoch), true, nil
}

// LowestSignedSourceEpoch is implemented only to satisfy the interface.
func (s *Store) LowestSignedSourceEpoch(_ context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) (primitives.Epoch, bool, error) {
	// Get validator slashing protection.
	validatorSlashingProtection, err := s.validatorSlashingProtection(pubKey)
	if err != nil {
		return 0, false, errors.Wrap(err, "could not get validator slashing protection")
	}

	// If there is no validator slashing protection, return early.
	if validatorSlashingProtection == nil {
		return 0, false, nil
	}

	// Return the lowest (and unique) signed source epoch.
	return primitives.Epoch(validatorSlashingProtection.LastSignedAttestationSourceEpoch), true, nil
}

// AttestedPublicKeys returns the list of public keys in the database.
func (s *Store) AttestedPublicKeys(_ context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
	// Retrieve all public keys in database.
	pubkeys, err := s.publicKeys()
	if err != nil {
		return nil, errors.Wrap(err, "could not get public keys")
	}

	// Filter public keys which already attested.
	attestedPublicKeys := make([][fieldparams.BLSPubkeyLength]byte, 0, len(pubkeys))
	for _, pubkey := range pubkeys {
		// Get validator slashing protection.
		validatorSlashingProtection, err := s.validatorSlashingProtection(pubkey)
		if err != nil {
			return nil, errors.Wrap(err, "could not get validator slashing protection")
		}

		// If there is no target epoch, return early.
		if validatorSlashingProtection == nil || validatorSlashingProtection.LastSignedAttestationTargetEpoch == nil {
			continue
		}

		// Append the attested public key.
		attestedPublicKeys = append(attestedPublicKeys, pubkey)
	}

	// Return the attested public keys.
	return attestedPublicKeys, nil
}

// SlashableAttestationCheck checks if an attestation is slashable by comparing it with the attesting
// history for the given public key in our minimal slashing protection database defined by EIP-3076.
// If it is not, it updates the database.
func (s *Store) SlashableAttestationCheck(
	ctx context.Context,
	indexedAtt ethpb.IndexedAtt,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	signingRoot32 [32]byte,
	_ bool,
	_ *prometheus.CounterVec,
) error {
	ctx, span := trace.StartSpan(ctx, "validator.postAttSignUpdate")
	defer span.End()

	// Check if the attestation is potentially slashable regarding EIP-3076 minimal conditions.
	// If not, save the new attestation into the database.
	if err := s.SaveAttestationForPubKey(ctx, pubKey, signingRoot32, indexedAtt); err != nil {
		if strings.Contains(err.Error(), "could not sign attestation") {
			return errors.Wrap(err, failedAttLocalProtectionErr)
		}

		return errors.Wrap(err, "could not save attestation history for validator public key")
	}

	return nil
}

// SaveAttestationForPubKey checks if the incoming attestation is valid regarding EIP-3076 minimal slashing protection.
// If so, it updates the database with the incoming source and target, and returns nil.
// If not, it does not modify the database and return an error.
func (s *Store) SaveAttestationForPubKey(
	_ context.Context,
	pubkey [fieldparams.BLSPubkeyLength]byte,
	_ [32]byte,
	att ethpb.IndexedAtt,
) error {
	// If there is no attestation, return on error.
	if att == nil || att.IsNil() || att.GetData().Source == nil || att.GetData().Target == nil {
		return errors.New("incoming attestation does not contain source and/or target epoch")
	}
	// Get validator slashing protection.
	validatorSlashingProtection, err := s.validatorSlashingProtection(pubkey)
	if err != nil {
		return errors.Wrap(err, "could not get validator slashing protection")
	}

	incomingSourceEpochUInt64 := uint64(att.GetData().Source.Epoch)
	incomingTargetEpochUInt64 := uint64(att.GetData().Target.Epoch)

	if validatorSlashingProtection == nil {
		// If there is no validator slashing protection, create one.
		validatorSlashingProtection = &ValidatorSlashingProtection{
			LastSignedAttestationSourceEpoch: incomingSourceEpochUInt64,
			LastSignedAttestationTargetEpoch: &incomingTargetEpochUInt64,
		}

		// Save the validator slashing protection.
		if err := s.saveValidatorSlashingProtection(pubkey, validatorSlashingProtection); err != nil {
			return errors.Wrap(err, "could not save validator slashing protection")
		}

		return nil
	}

	savedSourceEpoch := validatorSlashingProtection.LastSignedAttestationSourceEpoch
	savedTargetEpoch := validatorSlashingProtection.LastSignedAttestationTargetEpoch

	// Based on EIP-3076 (minimal database), validator should refuse to sign any attestation
	// with source epoch less than the recorded source epoch.
	if incomingSourceEpochUInt64 < savedSourceEpoch {
		return errors.Errorf(
			"could not sign attestation with source lower than recorded source epoch, %d < %d",
			att.GetData().Source.Epoch,
			validatorSlashingProtection.LastSignedAttestationSourceEpoch,
		)
	}

	// Based on EIP-3076 (minimal database), validator should refuse to sign any attestation
	// with target epoch less than or equal to the recorded target epoch.
	if savedTargetEpoch != nil && incomingTargetEpochUInt64 <= *savedTargetEpoch {
		return errors.Errorf(
			"could not sign attestation with target lower than or equal to recorded target epoch, %d <= %d",
			att.GetData().Target.Epoch,
			*savedTargetEpoch,
		)
	}

	// Update the latest signed source and target epoch.
	validatorSlashingProtection.LastSignedAttestationSourceEpoch = incomingSourceEpochUInt64
	validatorSlashingProtection.LastSignedAttestationTargetEpoch = &incomingTargetEpochUInt64

	// Save the validator slashing protection.
	if err := s.saveValidatorSlashingProtection(pubkey, validatorSlashingProtection); err != nil {
		return errors.Wrap(err, "could not save validator slashing protection")
	}

	return nil
}

// SaveAttestationsForPubKey saves the attestation history for a list of public keys WITHOUT checking if the incoming
// attestations are valid regarding EIP-3076 minimal slashing protection.
// For each public key, incoming sources and targets epochs are compared with
// recorded source and target epochs, and maximums are saved.
func (s *Store) SaveAttestationsForPubKey(
	_ context.Context,
	pubkey [fieldparams.BLSPubkeyLength]byte,
	_ [][]byte,
	atts []*ethpb.IndexedAttestation,
) error {
	// If there is no attestation, return early.
	if len(atts) == 0 {
		return nil
	}

	// Retrieve maximum source and target epoch.
	maxIncomingSourceEpoch, maxIncomingTargetEpoch, err := maxSourceTargetEpoch(atts)
	if err != nil {
		return errors.Wrap(err, "could not get maximum source and target epoch")
	}

	// Convert epochs to uint64.
	maxIncomingSourceEpochUInt64 := uint64(maxIncomingSourceEpoch)
	maxIncomingTargetEpochUInt64 := uint64(maxIncomingTargetEpoch)

	// Get validator slashing protection.
	validatorSlashingProtection, err := s.validatorSlashingProtection(pubkey)
	if err != nil {
		return errors.Wrap(err, "could not get validator slashing protection")
	}

	if validatorSlashingProtection == nil {
		// If there is no validator slashing protection, create one.
		validatorSlashingProtection = &ValidatorSlashingProtection{
			LastSignedAttestationSourceEpoch: maxIncomingSourceEpochUInt64,
			LastSignedAttestationTargetEpoch: &maxIncomingTargetEpochUInt64,
		}

		// Save the validator slashing protection.
		if err := s.saveValidatorSlashingProtection(pubkey, validatorSlashingProtection); err != nil {
			return errors.Wrap(err, "could not save validator slashing protection")
		}

		return nil
	}

	savedSourceEpochUInt64 := validatorSlashingProtection.LastSignedAttestationSourceEpoch
	savedTargetEpochUInt64 := validatorSlashingProtection.LastSignedAttestationTargetEpoch

	maxSourceEpochUInt64 := maxIncomingSourceEpochUInt64
	maxTargetEpochUInt64 := maxIncomingTargetEpochUInt64

	// Compare the maximum incoming source and target epochs with what we have recorded.
	if savedSourceEpochUInt64 > maxSourceEpochUInt64 {
		maxSourceEpochUInt64 = savedSourceEpochUInt64
	}

	if savedTargetEpochUInt64 != nil && *savedTargetEpochUInt64 > maxTargetEpochUInt64 {
		maxTargetEpochUInt64 = *savedTargetEpochUInt64
	}

	// Update the validator slashing protection.
	validatorSlashingProtection.LastSignedAttestationSourceEpoch = maxSourceEpochUInt64
	validatorSlashingProtection.LastSignedAttestationTargetEpoch = &maxTargetEpochUInt64

	// Save the validator slashing protection.
	if err := s.saveValidatorSlashingProtection(pubkey, validatorSlashingProtection); err != nil {
		return errors.Wrap(err, "could not save validator slashing protection")
	}

	return nil
}

// AttestationHistoryForPubKey returns the attestation history for a public key.
func (s *Store) AttestationHistoryForPubKey(
	_ context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
) ([]*common.AttestationRecord, error) {
	// Get validator slashing protection
	validatorSlashingProtection, err := s.validatorSlashingProtection(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not get validator slashing protection")
	}

	// If there is no validator slashing protection or no target epoch, return an empty slice.
	if validatorSlashingProtection == nil || validatorSlashingProtection.LastSignedAttestationTargetEpoch == nil {
		return []*common.AttestationRecord{}, nil
	}

	// Return the (unique) attestation record.
	return []*common.AttestationRecord{
		{
			PubKey: pubKey,
			Source: primitives.Epoch(validatorSlashingProtection.LastSignedAttestationSourceEpoch),
			Target: primitives.Epoch(*validatorSlashingProtection.LastSignedAttestationTargetEpoch),
		},
	}, nil
}

// maxSourceTargetEpoch gets the maximum source and target epoch from atts.
func maxSourceTargetEpoch(atts []*ethpb.IndexedAttestation) (primitives.Epoch, primitives.Epoch, error) {
	maxSourceEpoch := primitives.Epoch(0)
	maxTargetEpoch := primitives.Epoch(0)

	for _, att := range atts {
		if att == nil || att.Data == nil || att.Data.Source == nil || att.Data.Target == nil {
			return 0, 0, errors.New("incoming attestation does not contain source and/or target epoch")
		}

		if att.Data.Source.Epoch > maxSourceEpoch {
			maxSourceEpoch = att.Data.Source.Epoch
		}

		if att.Data.Target.Epoch > maxTargetEpoch {
			maxTargetEpoch = att.Data.Target.Epoch
		}
	}
	return maxSourceEpoch, maxTargetEpoch, nil
}
