package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// TestEndToEnd_MinimalConfig_WithStateDiff runs the standard minimal suite with the
// hierarchical state-diff storage backend enabled. It starts from a Fulu genesis (no
// in-test fork transition), so the only constraint on its length is the exit/withdrawal
// timeline: it runs 10 epochs with exit at epoch 4, the same compressed schedule as the
// main minimal test.
func TestEndToEnd_MinimalConfig_WithStateDiff(t *testing.T) {
	r := e2eMinimal(t, types.InitForkCfg(version.Fulu, version.Fulu, params.E2ETestConfig()),
		types.WithStateDiff(),
		types.WithEpochs(10),
		types.WithExitEpoch(4),
	)
	r.run()
}
