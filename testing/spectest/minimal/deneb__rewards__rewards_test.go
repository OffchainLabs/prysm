package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/deneb/rewards"
)

func TestMinimal_Deneb_Rewards(t *testing.T) {
	rewards.RunPrecomputeRewardsAndPenaltiesTests(t, "minimal")
}
