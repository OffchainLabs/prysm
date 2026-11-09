package beacon_api

import (
	"context"
	"strconv"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) domainData(ctx context.Context, epoch primitives.Epoch, domainType [4]byte) (*ethpb.DomainResponse, error) {
	// Get the genesis validator root
	genesis, err := c.genesisProvider.Genesis(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get genesis info")
	}

	genesisValidatorRoot, err := bytesutil.DecodeHexWithLength(genesis.GenesisValidatorsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode genesis validators root")
	}

	genesisTime := time.Time{}
	if genesis.GenesisTime != "" {
		seconds, err := strconv.ParseInt(genesis.GenesisTime, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse genesis time %s", genesis.GenesisTime)
		}
		genesisTime = time.Unix(seconds, 0)
	}

	return validatorHelpers.ComputeDomainData(epoch, domainType, genesisValidatorRoot, genesisTime)
}
