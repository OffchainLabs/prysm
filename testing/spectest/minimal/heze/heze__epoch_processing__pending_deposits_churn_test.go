package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/epoch_processing"
)

func TestMinimal_Heze_EpochProcessing_PendingDepositsChurn(t *testing.T) {
	epoch_processing.RunPendingDepositsChurnTests(t, "minimal")
}
