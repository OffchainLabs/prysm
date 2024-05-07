//go:build !develop

package params

import (
	"github.com/mohae/deepcopy"
)

// BeaconConfig retrieves beacon chain config.
func BeaconConfig() *BeaconChainConfig {
	return configs.getActive()
}

// OverrideBeaconConfig by replacing the config. The preferred pattern is to
// call BeaconConfig(), change the specific parameters, and then call
// OverrideBeaconConfig(c). Any subsequent calls to params.BeaconConfig() will
// return this new configuration.
func OverrideBeaconConfig(c *BeaconChainConfig) {
	c.InitializeForkSchedule()
	configs.active = c
}

// Copy returns a copy of the config object.
func (b *BeaconChainConfig) Copy() *BeaconChainConfig {
	config, ok := deepcopy.Copy(*b).(BeaconChainConfig)
	if !ok {
		panic("somehow deepcopy produced a BeaconChainConfig that is not of the same type as the original") // lint:nopanic -- This would only panic with an application misconfiguration and it should fail right away.
	}
	return &config
}
