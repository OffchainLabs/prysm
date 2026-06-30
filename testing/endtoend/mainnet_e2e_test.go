package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// Run mainnet e2e config with the current release validator against latest beacon node.
// Note: validator image can be pulled from the current release: gcr.io/offchainlabs/prysm/validator:stable while beacon node image can be used the local one (:latest)
func TestEndToEnd_MainnetConfig_ValidatorAtCurrentRelease(t *testing.T) {
	r := e2eMainnet(t, false, types.InitForkCfg(version.Bellatrix, version.Fulu, params.E2EMainnetTestConfig()))
	r.run()
}

func TestEndToEnd_MainnetConfig_MultiClient(t *testing.T) {
	e2eMainnet(t, true, types.InitForkCfg(version.Bellatrix, version.Electra, params.E2EMainnetTestConfig())).run()
}
