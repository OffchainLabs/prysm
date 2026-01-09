package params

import (
	"fmt"
)

type Option func(*BeaconChainConfig)

func WithGenesisValidatorsRoot(gvr [32]byte) Option {
	return func(cfg *BeaconChainConfig) {
		cfg.GenesisValidatorsRoot = gvr
		log.WithField("genesisValidatorsRoot", fmt.Sprintf("%#x", gvr)).Info("Setting genesis validators root")
	}
}
