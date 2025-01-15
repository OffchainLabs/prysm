package endtoend

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/endtoend/types"
)

func TestEndToEnd_MultiScenarioRun(t *testing.T) {
	runner := e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Deneb, params.E2ETestConfig()), types.WithEpochs(26))

	runner.config.Evaluators = scenarioEvals()
	runner.config.EvalInterceptor = runner.multiScenario
	runner.scenarioRunner()
}

func TestEndToEnd_MinimalConfig_Web3Signer(t *testing.T) {
	e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Deneb, params.E2ETestConfig()), types.WithRemoteSigner()).run()
}

func TestEndToEnd_MinimalConfig_Web3Signer_PersistentKeys(t *testing.T) {
	e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Deneb, params.E2ETestConfig()), types.WithRemoteSignerAndPersistentKeysFile()).run()
}

func TestEndToEnd_MinimalConfig_ValidatorRESTApi(t *testing.T) {
	e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Deneb, params.E2ETestConfig()), types.WithCheckpointSync(), types.WithValidatorRESTApi()).run()
}

func TestEndToEnd_ScenarioRun_EEOffline(t *testing.T) {
	t.Skip("TODO(#10242) Prysm is current unable to handle an offline e2e")
	runner := e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Deneb, params.E2ETestConfig()))

	runner.config.Evaluators = scenarioEvals()
	runner.config.EvalInterceptor = runner.eeOffline
	runner.scenarioRunner()
}
