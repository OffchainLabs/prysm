package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/electra/rewards"
)

func TestMinimal_Electra_Rewards(t *testing.T) {
	rewards.RunPrecomputeRewardsAndPenaltiesTests(t, "minimal")
}
