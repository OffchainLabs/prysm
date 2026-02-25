package validator

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestGetPayloadAttestations_BeforeGloasFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 10
	params.OverrideBeaconConfig(cfg)

	// State at slot 0 (epoch 0), which is before GloasForkEpoch 10.
	headState, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(0))

	vs := &Server{}
	result := vs.getPayloadAttestations(t.Context(), headState)
	require.Equal(t, true, result == nil)
}

func TestGetPayloadAttestations_AtGloasFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	headState, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(0))

	vs := &Server{}
	result := vs.getPayloadAttestations(t.Context(), headState)
	require.NotNil(t, result)
	require.Equal(t, 0, len(result))
}
