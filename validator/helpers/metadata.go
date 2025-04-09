package helpers

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/validator/db/iface"
	"github.com/prysmaticlabs/prysm/v5/validator/slashing-protection-history/format"
)

func ValidateMetadata(ctx context.Context, validatorDB iface.ValidatorDB, interchangeJSON *format.EIPSlashingProtectionFormat) error {
	// We need to ensure the version in the metadata field matches the one we support.
	version := interchangeJSON.Metadata.InterchangeFormatVersion
	if version != format.InterchangeFormatVersion {
		return fmt.Errorf(
			"slashing protection JSON version '%s' is not supported, wanted '%s'",
			version,
			format.InterchangeFormatVersion,
		)
	}

	// We need to verify the genesis validators root matches that of our chain data, otherwise
	// the imported slashing protection JSON was created on a different chain.
	gvr, err := RootFromHex(interchangeJSON.Metadata.GenesisValidatorsRoot)
	if err != nil {
		return fmt.Errorf("%#x is not a valid root: %w", interchangeJSON.Metadata.GenesisValidatorsRoot, err)
	}
	dbGvr, err := validatorDB.GenesisValidatorsRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "could not retrieve genesis validators root to db")
	}
	if dbGvr == nil {
		if err = validatorDB.SaveGenesisValidatorsRoot(ctx, gvr[:]); err != nil {
			return errors.Wrap(err, "could not save genesis validators root to db")
		}
		return nil
	}
	if !bytes.Equal(dbGvr, gvr[:]) {
		return errors.New("genesis validators root doesn't match the one that is stored in slashing protection db. " +
			"Please make sure you import the protection data that is relevant to the chain you are on")
	}
	return nil
}
