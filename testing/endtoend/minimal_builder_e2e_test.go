package endtoend

import (
	"testing"
)

func TestEndToEnd_Kurtosis_MinimalConfig_Builder(t *testing.T) {
	// Prerequisite for Kurtosis: Load images needed.
	LoadPrysmDockerImages(t)

	tests := []KurtosisTestSuites{
		{
			enclaveName:    "minimal-builder",
			configPath:     "testing/endtoend/network-config/minimal-builder.yaml",
			epochsToRun:    15,
			runSyncTest:    false,
			extraPlaybooks: []string{"builder.yaml"},
			skipPlaybooks: []string{
				"fee-recipient.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.enclaveName, func(t *testing.T) {
			tt.Run(t)
		})
	}
}
