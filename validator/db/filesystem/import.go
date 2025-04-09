package filesystem

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
	"github.com/prysmaticlabs/prysm/v5/validator/db/iface"
	"github.com/prysmaticlabs/prysm/v5/validator/helpers"
	"github.com/prysmaticlabs/prysm/v5/validator/slashing-protection-history/format"
)

// ImportStandardProtectionJSON takes in EIP-3076 compliant JSON file used for slashing protection
// by Ethereum validators and imports its data into Prysm's internal minimal representation of slashing
// protection in the validator client's database.
func (s *Store) ImportStandardProtectionJSON(ctx context.Context, r io.Reader) error {
	// Read the JSON file
	encodedJSON, err := io.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "could not read slashing protection JSON file")
	}

	// Unmarshal the JSON file
	interchangeJSON := &format.EIPSlashingProtectionFormat{}
	if err := json.Unmarshal(encodedJSON, interchangeJSON); err != nil {
		return errors.Wrap(err, "could not unmarshal slashing protection JSON file")
	}

	// If there is no data in the JSON file, we can return early.
	if interchangeJSON.Data == nil {
		return nil
	}

	// We validate the `MetadataV0` field of the slashing protection JSON file.
	if err := helpers.ValidateMetadata(ctx, s, interchangeJSON); err != nil {
		return errors.Wrap(err, "slashing protection JSON metadata was incorrect")
	}

	// Save blocks proposals and attestations into the database
	bar := common.InitializeProgressBar(len(interchangeJSON.Data), "Save blocks proposals and attestations:")
	for _, item := range interchangeJSON.Data {
		// Update progress bar
		if err := bar.Add(1); err != nil {
			return errors.Wrap(err, "could not update progress bar")
		}

		// If item is nil, skip
		if item == nil {
			continue
		}

		// Convert pubkey to bytes array
		pubkeyBytes, err := hexutil.Decode(item.Pubkey)
		if err != nil {
			return errors.Wrap(err, "could not decode public key from hex")
		}

		pubkey := ([fieldparams.BLSPubkeyLength]byte)(pubkeyBytes)

		// Block proposals
		if err := importBlockProposals(ctx, pubkey, item, s); err != nil {
			return errors.Wrap(err, "could not import block proposals")
		}

		// Attestations
		if err := importAttestations(ctx, pubkey, item, s); err != nil {
			return errors.Wrap(err, "could not import attestations")
		}
	}

	return nil
}

func importBlockProposals(ctx context.Context, pubkey [fieldparams.BLSPubkeyLength]byte, item *format.ProtectionData, validatorDB iface.ValidatorDB) error {
	for _, sb := range item.SignedBlocks {
		// If signing block is nil, return early
		if sb == nil {
			return nil
		}

		// Convert slot to primitives.Slot
		slot, err := helpers.SlotFromString(sb.Slot)
		if err != nil {
			return errors.Wrap(err, "could not convert slot to primitives.Slot")
		}

		// Save proposal if not slashable regarding EIP-3076 (minimal database)
		if err := validatorDB.SaveProposalHistoryForSlot(ctx, pubkey, slot, []byte{}); err != nil && !strings.Contains(err.Error(), "could not sign proposal") {
			return errors.Wrap(err, "could not save proposal history from imported JSON to database")
		}
	}

	return nil
}

func importAttestations(ctx context.Context, pubkey [fieldparams.BLSPubkeyLength]byte, item *format.ProtectionData, validatorDB iface.ValidatorDB) error {
	atts := make([]*ethpb.IndexedAttestation, len(item.SignedAttestations))
	for i := range item.SignedAttestations {
		// Get signed attestation
		sa := item.SignedAttestations[i]

		// Convert source epoch to primitives.Epoch
		source, err := helpers.EpochFromString(sa.SourceEpoch)
		if err != nil {
			return errors.Wrap(err, "could not convert source epoch to primitives.Epoch")
		}

		// Convert target epoch to primitives.Epoch
		target, err := helpers.EpochFromString(sa.TargetEpoch)
		if err != nil {
			return errors.Wrap(err, "could not convert target epoch to primitives.Epoch")
		}

		// Create indexed attestation
		att := &ethpb.IndexedAttestation{
			Data: &ethpb.AttestationData{
				Source: &ethpb.Checkpoint{
					Epoch: source,
				},
				Target: &ethpb.Checkpoint{
					Epoch: target,
				},
			},
		}

		atts[i] = att
	}

	// Save attestations
	if err := validatorDB.SaveAttestationsForPubKey(ctx, pubkey, [][]byte{}, atts); err != nil && !strings.Contains(err.Error(), "could not sign attestation") {
		return errors.Wrap(err, "could not save attestation record from imported JSON to database")
	}

	return nil
}
