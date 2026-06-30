package endtoend

import (
	"testing"
)

func TestEndToEnd_Kurtosis_MinimalConfig_PostMerge(t *testing.T) {
	// Prerequisite for Kurtosis: Load images needed.
	LoadPrysmDockerImages(t)

	tests := []KurtosisTestSuites{
		{
			enclaveName: "minimal-postmerge",
			configPath:  "testing/endtoend/network-config/minimal-postmerge.yaml",
			epochsToRun: 20,
			runSyncTest: true,
			// minimal-postmerge reaches Electra at epoch 10. Current assertoor generates slashings only for Electra and later.
			skipPlaybooks: []string{
				"slashings.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.enclaveName, func(t *testing.T) {
			tt.Run(t)
		})
	}
}
