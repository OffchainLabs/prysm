package beacon_api

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) domainData(ctx context.Context, epoch primitives.Epoch, domainType [4]byte) (*ethpb.DomainResponse, error) {
	// Get the fork version from the given epoch
	fork, err := params.Fork(epoch)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get fork version for epoch %d", epoch)
	}

	// Get the genesis validator root
	genesis, err := c.genesisProvider.Genesis(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get genesis info")
	}

	genesisValidatorRoot, err := bytesutil.DecodeHexWithLength(genesis.GenesisValidatorsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode genesis validators root")
	}

	signatureDomain, err := signing.Domain(fork, epoch, domainType, genesisValidatorRoot)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute signature domain")
	}

	return &ethpb.DomainResponse{SignatureDomain: signatureDomain}, nil
}
