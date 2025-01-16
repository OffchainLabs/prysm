package endtoend

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/endtoend/types"
)

func TestEndToEnd_MultiScenarioRun_Multiclient(t *testing.T) {
	cfg := types.InitForkCfg(version.Bellatrix, version.Deneb, params.E2EMainnetTestConfig())
	runner := e2eMainnet(t, false, true, cfg, types.WithEpochs(24))
	// override for scenario tests
	runner.config.Evaluators = scenarioEvalsMulti(cfg)
	runner.config.EvalInterceptor = runner.multiScenarioMulticlient
	runner.scenarioRunner()
}
