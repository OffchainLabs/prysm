package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/epoch_processing"
)

func TestMinimal_Heze_EpochProcessing_EffectiveBalanceUpdates(t *testing.T) {
	epoch_processing.RunEffectiveBalanceUpdatesTests(t, "minimal")
}
