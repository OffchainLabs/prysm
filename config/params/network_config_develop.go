//go:build develop

//go:test develop
package params

import (
	"sync"

	"github.com/mohae/deepcopy"
)

var networkConfigLock sync.Mutex

// BeaconNetworkConfig returns the current network config for
// the beacon chain.
func BeaconNetworkConfig() *NetworkConfig {
	networkConfigLock.Lock()
	defer networkConfigLock.Unlock()
	return networkConfig
}

// OverrideBeaconNetworkConfig will override the network
// config with the added argument.
func OverrideBeaconNetworkConfig(cfg *NetworkConfig) {
	networkConfigLock.Lock()
	defer networkConfigLock.Unlock()
	networkConfig = cfg.Copy()
}

// Copy returns Copy of the config object.
func (c *NetworkConfig) Copy() *NetworkConfig {
	config, ok := deepcopy.Copy(*c).(NetworkConfig)
	if !ok {
		networkConfigLock.Lock()
		config = *networkConfig
		networkConfigLock.Unlock()
	}
	return &config
}
