package eth1

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestCurrentBlobTxMode(t *testing.T) {
	cases := []struct {
		name   string
		epoch  primitives.Epoch
		cfg    func() *params.BeaconChainConfig
		wanted blobTxMode
	}{
		{
			name:  "pre deneb disables blob txs",
			epoch: 3,
			cfg: func() *params.BeaconChainConfig {
				cfg := params.E2ETestConfig().Copy()
				cfg.DenebForkEpoch = 4
				cfg.FuluForkEpoch = cfg.FarFutureEpoch
				return cfg
			},
			wanted: blobTxModeNone,
		},
		{
			name:  "deneb without fulu uses sidecars",
			epoch: 4,
			cfg: func() *params.BeaconChainConfig {
				cfg := params.E2ETestConfig().Copy()
				cfg.DenebForkEpoch = 4
				cfg.FuluForkEpoch = cfg.FarFutureEpoch
				return cfg
			},
			wanted: blobTxModeSidecar,
		},
		{
			name:  "fulu upgrades sidecars to cell proofs",
			epoch: 6,
			cfg: func() *params.BeaconChainConfig {
				cfg := params.E2ETestConfig().Copy()
				cfg.DenebForkEpoch = 4
				cfg.FuluForkEpoch = 6
				return cfg
			},
			wanted: blobTxModeCellProof,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wanted, currentBlobTxMode(tc.epoch, tc.cfg()))
		})
	}
}

func TestBPOBlobTxBudgetStillExceedsOldLimit(t *testing.T) {
	require.Equal(t, true, blobTxCount*6 > 9)
}
