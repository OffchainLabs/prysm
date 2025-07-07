package beacon_api

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

type GenesisProvider interface {
	Genesis(ctx context.Context) (*structs.Genesis, error)
}

type beaconApiGenesisProvider struct {
	jsonRestHandler RestHandler
	genesis         *structs.Genesis
	once            sync.Once
}

func (c *beaconApiValidatorClient) waitForChainStart(ctx context.Context) (*ethpb.ChainStartResponse, error) {
	genesis, err := c.genesisProvider.Genesis(ctx)

	for err != nil {
		jsonErr := &httputil.DefaultJsonError{}
		httpNotFound := errors.As(err, &jsonErr) && jsonErr.Code == http.StatusNotFound
		if !httpNotFound {
			return nil, errors.Wrap(err, "failed to get genesis data")
		}

		// Error 404 means that the chain genesis info is not yet known, so we query it every second until it's ready
		select {
		case <-time.After(time.Second):
			genesis, err = c.genesisProvider.Genesis(ctx)
		case <-ctx.Done():
			return nil, errors.New("context canceled")
		}
	}

	genesisTime, err := strconv.ParseUint(genesis.GenesisTime, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse genesis time: %s", genesis.GenesisTime)
	}

	genesisValidatorRoot, err := bytesutil.DecodeHexWithLength(genesis.GenesisValidatorsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode genesis validators root")
	}

	chainStartResponse := &ethpb.ChainStartResponse{
		Started:               true,
		GenesisTime:           genesisTime,
		GenesisValidatorsRoot: genesisValidatorRoot,
	}

	return chainStartResponse, nil
}

// GetGenesis gets the genesis information from the beacon node via the /eth/v1/beacon/genesis endpoint
func (c *beaconApiGenesisProvider) Genesis(ctx context.Context) (*structs.Genesis, error) {
	genesisJson := &structs.GetGenesisResponse{}
	var doErr error
	c.once.Do(func() {
		if err := c.jsonRestHandler.Get(ctx, "/eth/v1/beacon/genesis", genesisJson); err != nil {
			doErr = err
			return
		}
		if genesisJson.Data == nil {
			doErr = errors.New("genesis data is nil")
			return
		}
		c.genesis = genesisJson.Data
	})
	if doErr != nil {
		// Allow another call because the current one returned an error
		c.once = sync.Once{}
	}
	return c.genesis, doErr
}
