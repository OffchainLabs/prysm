//go:build !develop

//go:test !develop
package params

import "github.com/mohae/deepcopy"

// BeaconNetworkConfig returns the current network config for
// the beacon chain.
func BeaconNetworkConfig() *NetworkConfig {
	return networkConfig
}

// OverrideBeaconNetworkConfig will override the network
// config with the added argument.
func OverrideBeaconNetworkConfig(cfg *NetworkConfig) {
	networkConfig = cfg.Copy()
}

// Copy returns Copy of the config object.
func (c *NetworkConfig) Copy() *NetworkConfig {
	config, ok := deepcopy.Copy(*c).(NetworkConfig)
	if !ok {
		config = *networkConfig
	}
	return &config
}
