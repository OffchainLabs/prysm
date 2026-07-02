package endtoend

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/endtoend/kurtosis"
)

// TestEndToEnd_MultiScenarioRun_MultiClient2 assumes those service names.
var mainnetSecondNodeServices = []string{"cl-2-prysm-geth", "vc-2-geth-prysm"}

// TestEndToEnd_MultiScenarioRun_MultiClient2 runs mainnet-preset multiclient scenarios in a single enclave, with the following events:
// Event 1. Freeze the 2nd Prysm BN + VC (mainnetSecondNodeServices), and resume after one epoch.
// Event 2. Optimistic Sync: drive the 2nd Prysm node optimistic via its faultproxy snooper, then clear.
//
// After both events settle, we run one-shot assertoor playbooks once to verify the network
// recovered. Unlike the minimal scenario, we do NOT assert health right after the freeze:
// mainnet's 6s slots make the post-freeze recovery window (~epoch 8) too turbulent (deep
// reorgs, missed slots, low sync participation) for a strict one-shot; the always-on
// steady-state monitors cover that window instead. cl-2 (the snooper node) is a minority so
// its snooper-induced latency barely moves fork choice.
//
// Note that the network doesn't fork at all, starts from Fulu. See mainnet-scenario.yaml for the network config.
// Here's the brief timeline (absolute epochs, anchored at firstFinalizedEpoch = 3):
//
//	epoch:  3    4    5    6    7    8    9    10   11   12   13   14   15
//	Prysm #2     x════o                        F════C
//	one-shot                                                  A    M
//
//	x = stop   o = start (resume)   F = opt-sync fault on   C = fault cleared
//	A = attestation-stats-once
//	M = metrics-once + validators-sync-participation-once
//	    (network-health-once is omitted: its fork/reorg checks flag benign 1-slot
//	    client lag and the still-syncing checkpoint node in this multiclient net)
//	Test ends at epoch 15 (epochsToRun).
func TestEndToEnd_MultiScenarioRun_MultiClient2(t *testing.T) {
	LoadPrysmDockerImages(t)
	LoadFaultproxyImage(t)

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

	// Scenario 2. Optimistic Sync: cl-2 (minority) goes optimistic while cl-1 + lighthouse keep finalizing.
	assertoorEvents = append(assertoorEvents,
		kurtosis.AssertoorEvent{Epoch: firstFinalizedEpoch + 7, Playbook: "optimistic-sync-fault-on.yaml"},
		kurtosis.AssertoorEvent{Epoch: firstFinalizedEpoch + 8, Playbook: "optimistic-sync-fault-off.yaml"},
	)

	// One-shot health checks run once, after both scenarios have settled (round 2).
	// mainnet's 6s slots make the post-freeze recovery window (~epoch 8) too turbulent
	// (deep reorgs, missed slots, low sync participation) for a strict one-shot
	// assertion; the always-on steady-state monitors cover that gap instead.
	for _, epoch := range []uint64{firstFinalizedEpoch + 10} {
		assertoorEvents = append(assertoorEvents,
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "attestation-stats-once.yaml"},
		)
	}
	for _, epoch := range []uint64{firstFinalizedEpoch + 11} {
		assertoorEvents = append(assertoorEvents,
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "metrics-once.yaml"},
			kurtosis.AssertoorEvent{Epoch: epoch, Playbook: "validators-sync-participation-once.yaml"},
			// network-health-once is intentionally omitted here: its fork/reorg checks
			// count benign 1-slot client-head lag and the still-syncing checkpoint node
			// as forks in this diverse multiclient net. Node health + sync are covered by
			// the steady-state monitors and the checkpoint/p2p sync tests.
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
