package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

func TestEndToEnd_MultiScenarioRun(t *testing.T) {
	cfg := types.InitForkCfg(version.Bellatrix, version.Electra, params.E2ETestConfig())
	runner := e2eMinimal(t, cfg, types.WithEpochs(28))
	// override for scenario tests
	runner.config.Evaluators = scenarioEvals(cfg)
	runner.config.EvalInterceptor = runner.multiScenario
	runner.scenarioRunner()
}

// Note: Legacy UsePersistentKeyFile cannot be mimicked in Kurtosis-backed e2e tests
// unless we submit a PR for ethereum-package that supports `--remote-signer-keys-file` flag.
// Currently, ethereum-package ALWAYS starts Prysm remote signer with `--remote-signer-url`
// and `--remote-signer-keys`.
func TestEndToEnd_MinimalConfig_Web3Signer(t *testing.T) {
	LoadPrysmDockerImages(t)

	tests := []KurtosisTestSuites{
		{
			enclaveName: "minimal-web3signer",
			configPath:  "testing/endtoend/network-config/minimal-web3signer.yaml",
			epochsToRun: 20,
			runSyncTest: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.enclaveName, func(t *testing.T) {
			tt.Run(t)
		})
	}
}

func TestEndToEnd_MinimalConfig_CurrentFork(t *testing.T) {
	LoadPrysmDockerImages(t)

	tests := []KurtosisTestSuites{
		{
			enclaveName: "minimal-current-fork",
			configPath:  "testing/endtoend/network-config/minimal-current-fork.yaml",
			epochsToRun: 15,
			runSyncTest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.enclaveName, func(t *testing.T) {
			tt.Run(t)
		})
	}
}

// TestEndToEnd_Kurtosis_MinimalConfig_REST_SSZ runs the minimal e2e with validating VCs
// Replaces the legacy ValidatorRESTApi and ValidatorRESTApi_SSZ tests.
func TestEndToEnd_Kurtosis_MinimalConfig_REST_SSZ(t *testing.T) {
	// Prerequisite for Kurtosis: Load images needed.
	LoadPrysmDockerImages(t)

	tests := []KurtosisTestSuites{
		{
			enclaveName: "minimal-restapi",
			configPath:  "testing/endtoend/network-config/minimal-restapi.yaml",
			epochsToRun: 20,
			runSyncTest: true,
			// minimal-restapi reaches Electra at epoch 16. Current assertoor generates slashings only for Electra and later.
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

func TestEndToEnd_ScenarioRun_EEOffline(t *testing.T) {
	t.Skip("TODO(#10242) Prysm is current unable to handle an offline e2e")
	cfg := types.InitForkCfg(version.Bellatrix, version.Deneb, params.E2ETestConfig())
	runner := e2eMinimal(t, cfg)
	// override for scenario tests
	runner.config.Evaluators = scenarioEvals(cfg)
	runner.config.EvalInterceptor = runner.eeOffline
	runner.scenarioRunner()
}
