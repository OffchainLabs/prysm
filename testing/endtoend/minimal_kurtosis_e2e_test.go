package endtoend

import (
	"testing"
)

// TestEndToEnd_Kurtosis_MinimalConfig runs the e2e test with the minimal config in a Kurtosis enclave.
func TestEndToEnd_Kurtosis_MinimalConfig(t *testing.T) {
	// Prerequisite for Kurtosis: Load images needed.
	LoadPrysmDockerImages(t)

	testSuites := []KurtosisTestSuites{
		{
			enclaveName: "minimal",
			configPath:  "testing/endtoend/network-config/minimal.yaml",
			epochsToRun: 15,
			runSyncTest: true,
		},
		{
			enclaveName: "minimal-statediff",
			configPath:  "testing/endtoend/network-config/minimal-statediff.yaml",
			epochsToRun: 20,
			runSyncTest: true,
			// minimal-statediff reaches Electra at epoch 10. Current assertoor generates slashings only for Electra and later.
			skipPlaybooks: []string{
				"slashings.yaml",
			},
		},
	}

	for _, suite := range testSuites {
		t.Run(suite.enclaveName, func(t *testing.T) {
			suite.Run(t)
		})
	}
}
