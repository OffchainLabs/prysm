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

func TestEndToEnd_MinimalConfig_Web3Signer(t *testing.T) {
	e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Electra, params.E2ETestConfig()), types.WithRemoteSigner()).run()
}

func TestEndToEnd_MinimalConfig_Web3Signer_PersistentKeys(t *testing.T) {
	e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Electra, params.E2ETestConfig()), types.WithRemoteSignerAndPersistentKeysFile()).run()
}

func TestEndToEnd_MinimalConfig_CurrentFork(t *testing.T) {
	r := e2eMinimal(t, types.InitForkCfg(version.Electra, version.Electra, params.E2ETestConfig()), types.WithCheckpointSync())
	r.run()
}

func TestEndToEnd_MinimalConfig_ValidatorRESTApi_SSZ(t *testing.T) {
	e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Electra, params.E2ETestConfig()), types.WithCheckpointSync(), types.WithValidatorRESTApi(), types.WithSSZOnly()).run()
}

func TestEndToEnd_MinimalConfig_ValidatorRESTApi(t *testing.T) {
	e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Electra, params.E2ETestConfig()), types.WithCheckpointSync(), types.WithValidatorRESTApi()).run()
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

// TestEndToEnd_PreFuluSemiSupernodeRestart tests the bug where restarting a beacon node
// with --semi-supernode flag before the Fulu fork causes earliestAvailableSlot to decrease.
//
// Bug scenario being tested:
// 1. Node starts with Fulu scheduled but not active - uses EarliestSlot() for earliestAvailableSlot
// 2. Validators connect - maintainCustodyInfo() updates earliestAvailableSlot to headSlot (higher)
// 3. Restart with --semi-supernode (still before Fulu) - EarliestSlot() returns checkpoint slot
//    BUG: The lower checkpoint slot should NOT overwrite the higher headSlot value
//
// The test verifies that earliestAvailableSlot is monotonically non-decreasing.
func TestEndToEnd_PreFuluSemiSupernodeRestart(t *testing.T) {
	// Configure with Fulu scheduled at epoch 6
	// This gives us epochs 0-5 to run the scenario before Fulu activates:
	// - Epoch 3: Record initial custody info
	// - Epoch 4: Validators connected, custody info updated
	// - Epoch 5: Restart with --semi-supernode
	// - Epoch 6: Fulu activates, verify earliestAvailableSlot did not decrease
	const fuluForkEpoch = 6
	cfg := types.InitForkCfgWithFuluAt(version.Electra, fuluForkEpoch, params.E2ETestConfig())

	// Run for enough epochs to complete the test (through Fulu fork + 1 recovery epoch)
	// Epochs 0-2: warmup, 3-5: scenario phases, 6: verify, 7: recovery
	// Use checkpoint sync so earliestAvailableSlot starts at a non-zero value
	runner := e2eMinimal(t, cfg, types.WithEpochs(8), types.WithCheckpointSync())

	// Override for this specific scenario test
	runner.config.Evaluators = scenarioEvals(cfg)
	runner.config.EvalInterceptor = runner.preFuluSemiSupernodeRestart
	runner.scenarioRunner()
}
