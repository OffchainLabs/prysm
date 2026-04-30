package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/forkchoice"
)

func TestMinimal_Fulu_ForkchoiceCompliance_BlockWeight(t *testing.T) {
	forkchoice.RunComplianceSuite(t, "minimal", version.Fulu, "block_weight_test")
}
