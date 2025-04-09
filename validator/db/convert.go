package db

import (
	"context"
	"path/filepath"

	"github.com/pkg/errors"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
	"github.com/prysmaticlabs/prysm/v5/validator/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/validator/db/iface"
	"github.com/prysmaticlabs/prysm/v5/validator/db/kv"
)

// ConvertDatabase converts a minimal database to a complete database or a complete database to a minimal database.
// Delete the source database after conversion.
func ConvertDatabase(ctx context.Context, sourceDataDir string, targetDataDir string, minimalToComplete bool) error {
	// Check if the source database exists.
	var (
		sourceDatabaseExists bool
		err                  error
	)

	if minimalToComplete {
		sourceDataBasePath := filepath.Join(sourceDataDir, filesystem.DatabaseDirName)
		sourceDatabaseExists, err = file.Exists(sourceDataBasePath, file.Directory)
	} else {
		sourceDataBasePath := filepath.Join(sourceDataDir, kv.ProtectionDbFileName)
		sourceDatabaseExists, err = file.Exists(sourceDataBasePath, file.Regular)
	}

	if err != nil {
		return errors.Wrap(err, "could not check if source database exists")
	}

	// If the source database does not exist, there is nothing to convert.
	if !sourceDatabaseExists {
		return errors.New("source database does not exist")
	}

	// Get the source database.
	var sourceDatabase iface.ValidatorDB

	if minimalToComplete {
		sourceDatabase, err = filesystem.NewStore(sourceDataDir, nil)
	} else {
		sourceDatabase, err = kv.NewKVStore(ctx, sourceDataDir, nil)
	}

	if err != nil {
		return errors.Wrap(err, "could not get source database")
	}

	// Close the source database.
	defer func() {
		if err := sourceDatabase.Close(); err != nil {
			log.WithError(err).Error("Failed to close source database")
		}
	}()

	// Create the target database.
	var targetDatabase iface.ValidatorDB

	if minimalToComplete {
		targetDatabase, err = kv.NewKVStore(ctx, targetDataDir, nil)
	} else {
		targetDatabase, err = filesystem.NewStore(targetDataDir, nil)
	}

	if err != nil {
		return errors.Wrap(err, "could not create target database")
	}

	// Close the target database.
	defer func() {
		if err := targetDatabase.Close(); err != nil {
			log.WithError(err).Error("Failed to close target database")
		}
	}()

	// Genesis
	// -------
	// Get the genesis validators root.
	genesisValidatorRoot, err := sourceDatabase.GenesisValidatorsRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get genesis validators root from source database")
	}

	// Save the genesis validators root.
	if err := targetDatabase.SaveGenesisValidatorsRoot(ctx, genesisValidatorRoot); err != nil {
		return errors.Wrap(err, "could not save genesis validators root")
	}

	// Graffiti
	// --------
	// Get the graffiti file hash.
	graffitiFileHash, exists, err := sourceDatabase.GraffitiFileHash()
	if err != nil {
		return errors.Wrap(err, "could not get graffiti file hash from source database")
	}

	if exists {
		// Calling GraffitiOrderedIndex will save the graffiti file hash.
		if _, err := targetDatabase.GraffitiOrderedIndex(ctx, graffitiFileHash); err != nil {
			return errors.Wrap(err, "could get graffiti ordered index")
		}
	}

	// Get the graffiti ordered index.
	graffitiOrderedIndex, err := sourceDatabase.GraffitiOrderedIndex(ctx, graffitiFileHash)
	if err != nil {
		return errors.Wrap(err, "could not get graffiti ordered index from source database")
	}

	// Save the graffiti ordered index.
	if err := targetDatabase.SaveGraffitiOrderedIndex(ctx, graffitiOrderedIndex); err != nil {
		return errors.Wrap(err, "could not save graffiti ordered index")
	}

	// Proposer settings
	// -----------------
	// Get the proposer settings.
	proposerSettings, err := sourceDatabase.ProposerSettings(ctx)

	switch {
	case err == nil:
		// Save the proposer settings.
		if err := targetDatabase.SaveProposerSettings(ctx, proposerSettings); err != nil {
			return errors.Wrap(err, "could not save proposer settings")
		}
	case errors.Is(err, kv.ErrNoProposerSettingsFound), errors.Is(err, filesystem.ErrNoProposerSettingsFound):
		// Nothing to do.
	default:
		return errors.Wrap(err, "could not get proposer settings from source database")
	}

	// Attestations
	// ------------
	// Get all public keys that have attested.
	attestedPublicKeys, err := sourceDatabase.AttestedPublicKeys(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get attested public keys from source database")
	}

	// Initialize the progress bar.
	bar := common.InitializeProgressBar(
		len(attestedPublicKeys),
		"Processing attestations:",
	)

	for _, pubkey := range attestedPublicKeys {
		// Update the progress bar.
		if err := bar.Add(1); err != nil {
			log.WithError(err).Debug("Could not increase progress bar")
		}

		// Get the attestation records.
		attestationRecords, err := sourceDatabase.AttestationHistoryForPubKey(ctx, pubkey)
		if err != nil {
			return errors.Wrap(err, "could not get attestation history for public key")
		}

		// If there are no attestation records, skip this public key.
		if len(attestationRecords) == 0 {
			continue
		}

		highestSource, highestTarget := primitives.Epoch(0), primitives.Epoch(0)
		for _, record := range attestationRecords {
			// If the record is nil, skip it.
			if record == nil {
				continue
			}

			// Get the highest source and target epoch.
			if record.Source > highestSource {
				highestSource = record.Source
			}

			if record.Target > highestTarget {
				highestTarget = record.Target
			}
		}

		// Create the indexed attestation with the highest source and target epoch.
		indexedAttestation := &ethpb.IndexedAttestation{
			Data: &ethpb.AttestationData{
				Source: &ethpb.Checkpoint{
					Epoch: highestSource,
				},
				Target: &ethpb.Checkpoint{
					Epoch: highestTarget,
				},
			},
		}

		if err := targetDatabase.SaveAttestationForPubKey(ctx, pubkey, [fieldparams.RootLength]byte{}, indexedAttestation); err != nil {
			return errors.Wrap(err, "could not save attestation for public key")
		}
	}

	// Proposals
	// ---------
	// Get all pubkeys in database.
	proposedPublicKeys, err := sourceDatabase.ProposedPublicKeys(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get proposed public keys from source database")
	}

	// Initialize the progress bar.
	bar = common.InitializeProgressBar(
		len(attestedPublicKeys),
		"Processing proposals:",
	)

	for _, pubkey := range proposedPublicKeys {
		// Update the progress bar.
		if err := bar.Add(1); err != nil {
			log.WithError(err).Debug("Could not increase progress bar")
		}

		// Get the proposal history.
		proposals, err := sourceDatabase.ProposalHistoryForPubKey(ctx, pubkey)
		if err != nil {
			return errors.Wrap(err, "could not get proposal history for public key")
		}

		// If there are no proposals, skip this public key.
		if len(proposals) == 0 {
			continue
		}

		highestSlot := primitives.Slot(0)
		for _, proposal := range proposals {
			// If proposal is nil, skip it.
			if proposal == nil {
				continue
			}

			// Get the highest slot.
			if proposal.Slot > highestSlot {
				highestSlot = proposal.Slot
			}
		}

		// Save the proposal history for the highest slot.
		if err := targetDatabase.SaveProposalHistoryForSlot(ctx, pubkey, highestSlot, nil); err != nil {
			return errors.Wrap(err, "could not save proposal history for public key")
		}
	}

	// Delete the source database.
	if err := sourceDatabase.ClearDB(); err != nil {
		return errors.Wrap(err, "could not delete source database")
	}

	return nil
}
