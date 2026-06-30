package endtoend

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// Run mainnet e2e config with the current release validator against latest beacon node.
// Note: validator image can be pulled from the current release: gcr.io/offchainlabs/prysm/validator:stable while beacon node image can be used the local one (:latest)
func TestEndToEnd_MainnetConfig_ValidatorAtCurrentRelease(t *testing.T) {
	r := e2eMainnet(t, false, types.InitForkCfg(version.Bellatrix, version.Fulu, params.E2EMainnetTestConfig()))
	r.run()
}

func TestEndToEnd_MainnetConfig_MultiClient(t *testing.T) {
	// Prerequisite for Kurtosis: Load images needed.
	LoadPrysmDockerImages(t)

	tests := []KurtosisTestSuites{
		{
			enclaveName: "mainnet-multiclient",
			configPath:  "testing/endtoend/network-config/mainnet-multiclient.yaml",
			// Total test duration = 10 epochs * 6 seconds/slot * 32 slots/epoch = 1920 seconds = 32 minutes
			epochsToRun:       10,
			runSyncTest:       true,
			lateSyncNodeDelay: 10 * time.Minute,
			skipPlaybooks: []string{
				"block-graffiti.yaml",

				// Skip all validator lifecycle tests.
				"deposits.yaml",
				"slashings.yaml",
				"voluntary-exits.yaml",
				"withdrawals.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.enclaveName, func(t *testing.T) {
			tt.Run(t)
		})
	}
}
