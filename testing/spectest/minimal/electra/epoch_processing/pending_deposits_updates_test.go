package epoch_processing

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/electra/epoch_processing"
)

func TestMinimal_Electra_EpochProcessing_PendingDeposits(t *testing.T) {
	epoch_processing.RunPendingDepositsTests(t, "minimal")
}
