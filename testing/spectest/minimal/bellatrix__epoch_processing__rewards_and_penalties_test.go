package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/bellatrix/epoch_processing"
)

func TestMinimal_Bellatrix_EpochProcessing_RewardsAndPenalties(t *testing.T) {
	epoch_processing.RunRewardsAndPenaltiesTests(t, "minimal")
}
