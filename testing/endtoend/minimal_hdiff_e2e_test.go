package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

func TestEndToEnd_MinimalConfig_WithStateDiff(t *testing.T) {
	r := e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Electra, params.E2ETestConfig()),
		types.WithStateDiff(),
	)
	r.run()
}
