package endtoend

import (
	"testing"
)

// TestEndToEnd_Kurtosis_MinimalConfig mirrors TestEndToEnd_MinimalConfig, but runs the test in a Kurtosis enclave instead of locally.
func TestEndToEnd_Kurtosis_MinimalConfig(t *testing.T) {
	// Prerequisite for Kurtosis: Load images needed.
	LoadPrysmDockerImages(t)

	testSuites := []KurtosisTestSuites{
		{
			enclaveName: "minimal",
			configPath:  "testing/endtoend/network-config/minimal.yaml",
			epochsToRun: 15,
		},
	}

	for _, suite := range testSuites {
		t.Run(suite.enclaveName, func(t *testing.T) {
			suite.Run(t)
		})
	}
}
