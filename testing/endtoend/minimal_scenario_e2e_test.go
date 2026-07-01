package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/kurtosis"
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

// TestEndToEnd_MultiScenarioRun assumes those service names.
var (
	secondNodeServices   = []string{"cl-2-prysm-geth", "vc-2-geth-prysm"}
	allValidatorServices = []string{"vc-1-geth-prysm", "vc-2-geth-prysm"}
)

// TestEndToEnd_MultiScenarioRun runs the specified scenarios in a single enclave, with the following events:
// Event 1. Freeze the 2nd BN + VC (secondNodeServices), and resume after one epoch.
// Event 2. Freeze all validator clients (allValidatorServices), and resume after one epoch.
// Event 3. Optimistic Sync with rpc snooper (TODO).
//
// Between each events, we wait for two epochs for recovery, and run one-shot assertoor playbooks
// to verify whether the recovery is successful.
//
// Note that the network doesn't fork at all, starts from Fulu. See minimal-scenario.yaml for the network config.
// Here's the brief timeline (absolute epochs, anchored at firstFinalizedEpoch = 3):
//
//	epoch:  3    4    5    6    7    8    9    10   11   12   13   14   15
//	BN #2        x════o
//	all VCs                          x════o
//	one-shot               A    M              A    M
//
//	x = stop   o = start (resume)
//	A = attestation-stats-once
//	M = metrics-once + validators-sync-participation-once + network-health-once
//	Test ends at epoch 15 (epochsToRun).
//
// Note for migration: Legacy e2e test corresponding to this starts from Bellatrix and upgrades until Electra,
// and then runs the same scenarios. Actually we do not need to test the fork upgrade here, as
// other e2e tests already cover the fork upgrade. Focusing on the main purpose of this test,
// which is to test the service stop/start scenarios, we can start from Fulu and never fork.
func TestEndToEnd_MultiScenarioRun2(t *testing.T) {
	LoadPrysmDockerImages(t)

	var (
		// serviceEvents defines the service stop/start events to simulate failures and recoveries.
		serviceEvents []kurtosis.EpochServiceEvent
		// assertoorEvents defines the assertoor playbooks for one-shot.
		assertoorEvents []kurtosis.AssertoorEvent
	)

	// Anchor epoch for the test.
	firstFinalizedEpoch := uint64(3)

	// Scenario 1. Freeze the second beacon node, and resume after one epoch.
	serviceEvents = append(serviceEvents,
		kurtosis.EpochServiceEvent{Epoch: firstFinalizedEpoch + 1, Action: kurtosis.ServiceStop, Services: secondNodeServices},
		kurtosis.EpochServiceEvent{Epoch: firstFinalizedEpoch + 2, Action: kurtosis.ServiceStart, Services: secondNodeServices},
	)

	// Scenario 2. Freeze all validator clients, and resume after one epoch.
	serviceEvents = append(serviceEvents,
		kurtosis.EpochServiceEvent{Epoch: firstFinalizedEpoch + 5, Action: kurtosis.ServiceStop, Services: allValidatorServices},
		kurtosis.EpochServiceEvent{Epoch: firstFinalizedEpoch + 6, Action: kurtosis.ServiceStart, Services: allValidatorServices},
	)

	// Scenario 3. Optimistic Sync.
	// TODO.

	// Schedule attestation stats one-shot playbook. See timeline above for the epochs.
	for _, epoch := range []uint64{firstFinalizedEpoch + 3, firstFinalizedEpoch + 7} {
		assertoorEvents = append(assertoorEvents,
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "attestation-stats-once.yaml"},
		)
	}
	// Schedule other one-shot playbooks. See timeline above for the epochs.
	for _, epoch := range []uint64{firstFinalizedEpoch + 4, firstFinalizedEpoch + 8} {
		assertoorEvents = append(assertoorEvents,
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "metrics-once.yaml"},
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "validators-sync-participation-once.yaml"},
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "network-health-once.yaml"},
		)
	}

	tests := []KurtosisTestSuites{
		{
			enclaveName: "minimal-scenario",
			configPath:  "testing/endtoend/network-config/minimal-scenario.yaml",
			epochsToRun: 15,
			runSyncTest: true,
			skipPlaybooks: []string{
				// Skip validator lifecycle playbooks.
				"deposits.yaml",
				"slashings.yaml",
				"withdrawals.yaml",

				// Skip long-running playbooks as they will fail due to the service stop/start events.
				"metrics.yaml",
				"network-health-monitor.yaml",
				"validators-sync-participation.yaml",
			},
			serviceEvents:   serviceEvents,
			assertoorEvents: assertoorEvents,
		},
	}

	for _, tt := range tests {
		t.Run(tt.enclaveName, func(t *testing.T) {
			tt.Run(t)
		})
	}
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
