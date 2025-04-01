package shared_providers

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
)

type genesisProvider struct {
	jsonRestHandler client.JsonRestHandler
	genesis         *structs.Genesis
	once            sync.Once
}

// GetGenesis gets the genesis information from the beacon node via the /eth/v1/beacon/genesis endpoint
func (c *genesisProvider) Genesis(ctx context.Context) (*structs.Genesis, error) {
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
