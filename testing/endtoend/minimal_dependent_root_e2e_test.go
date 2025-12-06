package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	ev "github.com/OffchainLabs/prysm/v7/testing/endtoend/evaluators"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// TestEndToEnd_MinimalConfig_DependentRoot tests that the beacon node correctly
// handles the dependent root bug scenario where:
// 1. A block at the first slot of an epoch becomes finalized
// 2. The parent of that block is pruned during finalization
// 3. Dependent root queries for that epoch would return wrong data without the fix
//
// The test adds verification logging that detects when a dependent root is
// returned from the wrong epoch (E2E_DEPENDENT_ROOT_BUG).
//
// WITH the fix: No bug is detected → test PASSES
// WITHOUT the fix: Bug is detected → test FAILS
func TestEndToEnd_MinimalConfig_DependentRoot(t *testing.T) {
	r := e2eMinimalDependentRoot(t, types.InitForkCfg(version.Electra, version.Electra, params.E2ETestConfig()))
	r.run()
}

func e2eMinimalDependentRoot(t *testing.T, cfg *params.BeaconChainConfig) *testRunner {
	// Use the standard e2eMinimal setup with the dependent root evaluators
	r := e2eMinimal(t, cfg,
		func(c *types.E2EConfig) {
			// Add the dependent root evaluators to check for the bug scenario
			c.Evaluators = append(c.Evaluators, ev.DependentRootEvaluators(2)...)
		},
	)
	return r
}
