package partialdatacolumnbroadcaster

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestNewBroadcasterRespectsConfigOverride verifies that NewBroadcaster picks
// up the current BeaconConfig().DataColumnSidecarSubnetCount, not a stale
// value captured at package init time.
//
// The current code uses package-level vars:
//
//	var maxConcurrentValidators = params.BeaconConfig().DataColumnSidecarSubnetCount
//
// which freezes the value at init. If a test (or a future config change)
// modifies BeaconConfig before creating a broadcaster, the semaphore size
// won't reflect it.
func TestNewBroadcasterRespectsConfigOverride(t *testing.T) {
	// Save and restore the original config.
	origConfig := params.BeaconConfig().Copy()
	defer params.OverrideBeaconConfig(origConfig)

	// Override DataColumnSidecarSubnetCount to a distinctive value.
	cfg := params.BeaconConfig().Copy()
	cfg.DataColumnSidecarSubnetCount = 42
	params.OverrideBeaconConfig(cfg)

	b := NewBroadcaster(nil)

	// The semaphore capacity should match the overridden config value.
	gotValidatorCap := cap(b.concurrentValidatorSemaphore)
	gotHeaderCap := cap(b.concurrentHeaderHandlerSemaphore)

	require.Equal(t, 42, gotValidatorCap,
		"concurrentValidatorSemaphore should use current config, not init-time value")
	require.Equal(t, 42, gotHeaderCap,
		"concurrentHeaderHandlerSemaphore should use current config, not init-time value")
}
