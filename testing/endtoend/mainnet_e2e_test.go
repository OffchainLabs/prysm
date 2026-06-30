package endtoend

import (
	"testing"
	"time"
)

// Run mainnet e2e config with the current release validator against latest beacon node.
func TestEndToEnd_MainnetConfig_ValidatorAtCurrentRelease(t *testing.T) {
	t.Parallel()

	// Prerequisite for Kurtosis: Load images needed.
	LoadPrysmDockerImages(t)

	tests := []KurtosisTestSuites{
		{
			enclaveName: "mainnet-stable-validator-release",
			configPath:  "testing/endtoend/network-config/mainnet-stable-validator-release.yaml",
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

func TestEndToEnd_MainnetConfig_MultiClient(t *testing.T) {
	t.Parallel()

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
