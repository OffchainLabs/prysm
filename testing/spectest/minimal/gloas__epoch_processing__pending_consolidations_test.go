package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/epoch_processing"
)

func TestMinimal_Gloas_EpochProcessing_PendingConsolidations(t *testing.T) {
	epoch_processing.RunPendingConsolidationsTests(t, "minimal")
}
