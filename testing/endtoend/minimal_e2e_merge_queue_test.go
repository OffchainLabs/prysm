package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// TestEndToEnd_MinimalConfig_MergeQueue runs a comprehensive e2e test with
// full fork transitions for merge queue validation.
func TestEndToEnd_MinimalConfig_MergeQueue(t *testing.T) {
	r := e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Electra, params.E2ETestConfig()), types.WithCheckpointSync())
	r.run()
}
