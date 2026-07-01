package endtoend

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/kurtosis"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

func TestEndToEnd_MultiScenarioRun_Multiclient(t *testing.T) {
	cfg := types.InitForkCfg(version.Bellatrix, version.Electra, params.E2EMainnetTestConfig())
	runner := e2eMainnet(t, true, cfg, types.WithEpochs(26))
	// override for scenario tests
	runner.config.Evaluators = scenarioEvalsMulti(cfg)
	runner.config.EvalInterceptor = runner.multiScenarioMulticlient
	runner.scenarioRunner()
}

// TestEndToEnd_MultiScenarioRun_MultiClient2 assumes those service names.
var mainnetSecondNodeServices = []string{"cl-2-prysm-geth", "vc-2-geth-prysm"}

// TestEndToEnd_MultiScenarioRun_MultiClient2 runs mainnet-preset scenarios in a single enclave, with the following events:
// Event 1. Freeze the 2nd Prysm BN + VC (mainnetSecondNodeServices), and resume after one epoch.
// Event 2. Optimistic Sync with late-start Prysm nodes.
//
// Between each event, we wait for two epochs for recovery, and run one-shot assertoor playbooks
// to verify whether the recovery is successful.
//
// Note that the network doesn't fork at all, starts from Fulu. See mainnet-scenario.yaml for the network config.
// Here's the brief timeline (absolute epochs, anchored at firstFinalizedEpoch = 3):
//
//	epoch:  3    4    5    6    7    8    9    10   11   12   13   14   15
//	Prysm #2     x====o
//	one-shot              A    M
//	late sync         x====o
//
//	x = stop   o = start (resume)
//	A = attestation-stats-once
//	M = metrics-once + validators-sync-participation-once + network-health-once
//	Test ends at epoch 15 (epochsToRun).
func TestEndToEnd_MultiScenarioRun_MultiClient2(t *testing.T) {
	LoadPrysmDockerImages(t)

	var (
		serviceEvents   []kurtosis.EpochServiceEvent
		assertoorEvents []kurtosis.AssertoorEvent
	)

	// Anchor epoch for the test.
	firstFinalizedEpoch := uint64(3)

	// Scenario 1. Freeze the second beacon node, and resume after one epoch.
	serviceEvents = append(serviceEvents,
		kurtosis.EpochServiceEvent{Epoch: firstFinalizedEpoch + 1, Action: kurtosis.ServiceStop, Services: mainnetSecondNodeServices},
		kurtosis.EpochServiceEvent{Epoch: firstFinalizedEpoch + 2, Action: kurtosis.ServiceStart, Services: mainnetSecondNodeServices},
	)

	// Scenario 2. Optimistic Sync.
	// TODO.

	// Schedule attestation stats one-shot playbook. See timeline above for the epochs.
	for _, epoch := range []uint64{firstFinalizedEpoch + 3} {
		assertoorEvents = append(assertoorEvents,
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "attestation-stats-once.yaml"},
		)
	}
	// Schedule other one-shot playbooks. See timeline above for the epochs.
	for _, epoch := range []uint64{firstFinalizedEpoch + 4} {
		assertoorEvents = append(assertoorEvents,
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "metrics-once.yaml"},
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "validators-sync-participation-once.yaml"},
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "network-health-once.yaml"},
		)
	}

	tests := []KurtosisTestSuites{
		{
			enclaveName: "mainnet-scenario",
			configPath:  "testing/endtoend/network-config/mainnet-scenario.yaml",
			epochsToRun: 15,
			runSyncTest: true,
			// Start the late sync nodes after finalization (~epoch 4 at 192s/epoch).
			lateSyncNodeDelay: 15 * time.Minute,
			skipPlaybooks: []string{
				// Skip graffiti check as we don't make LH produce graffiti in this test.
				"block-graffiti.yaml",

				// Skip validator lifecycle playbooks.
				"deposits.yaml",
				"slashings.yaml",
				"voluntary-exits.yaml",
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
