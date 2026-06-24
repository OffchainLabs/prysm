package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// TestEndToEnd_MinimalConfig_PostMerge is a post-submit test that runs the full
// e2e test from Bellatrix (post-merge) through all fork transitions to the latest fork.
// This test takes longer but provides comprehensive coverage of all forks.
func TestEndToEnd_MinimalConfig_PostMerge(t *testing.T) {
	r := e2eMinimal(t, types.InitForkCfg(version.Bellatrix, version.Fulu, params.E2ETestConfig()), types.WithCheckpointSync())
	r.run()
}
