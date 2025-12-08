package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// TestEndToEnd_MinimalConfig runs a shorter e2e test for presubmit feedback.
// Starts from the current fork without fork transitions (8 epochs).
func TestEndToEnd_MinimalConfig(t *testing.T) {
	r := e2eMinimal(t, types.InitForkCfg(version.Electra, version.Electra, params.E2ETestConfig()), types.WithCheckpointSync(), types.WithEpochs(8))
	r.run()
}