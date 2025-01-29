package endtoend

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/endtoend/types"
)

// Run mainnet e2e config with the current release validator against latest beacon node.
func TestEndToEnd_MainnetConfig_ValidatorAtCurrentRelease(t *testing.T) {
	r := e2eMainnet(t, true, false, types.InitForkCfg(version.Bellatrix, version.Deneb, params.E2EMainnetTestConfig()))
	r.run()
}

func TestEndToEnd_MainnetConfig_MultiClient(t *testing.T) {
	e2eMainnet(t, false, true, types.InitForkCfg(version.Bellatrix, version.Deneb, params.E2EMainnetTestConfig()), types.WithValidatorCrossClient()).run()
}
