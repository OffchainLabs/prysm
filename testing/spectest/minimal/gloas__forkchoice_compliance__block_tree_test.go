package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/forkchoice"
)

func TestMinimal_Gloas_ForkchoiceCompliance_BlockTree(t *testing.T) {
	forkchoice.RunComplianceSuite(t, "minimal", version.Gloas, "block_tree_test")
}
