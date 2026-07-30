package endtoend

import (
	"testing"
)

func TestEndToEnd_Kurtosis_MinimalConfig(t *testing.T) {
	// Prerequisite for Kurtosis: Load images needed.
	LoadPrysmDockerImages(t)

	testSuites := []KurtosisTestSuites{
		{
			enclaveName: "minimal",
			configPath:  "testing/endtoend/network-config/minimal.yaml",
			epochsToRun: 5,
		},
	}

	for _, suite := range testSuites {
		t.Run(suite.enclaveName, func(t *testing.T) {
			suite.Run(t)
		})
	}
}
