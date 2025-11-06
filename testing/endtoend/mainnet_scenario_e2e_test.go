package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

func TestEndToEnd_MultiScenarioRun_Multiclient(t *testing.T) {
	cfg := types.InitForkCfg(version.Bellatrix, version.Electra, params.E2EMainnetTestConfig())
	runner := e2eMainnet(t, false, true, cfg, types.WithEpochs(26))
	// override for scenario tests
	runner.config.Evaluators = scenarioEvalsMulti(cfg)
	runner.config.EvalInterceptor = runner.multiScenarioMulticlient
	runner.scenarioRunner()
}
