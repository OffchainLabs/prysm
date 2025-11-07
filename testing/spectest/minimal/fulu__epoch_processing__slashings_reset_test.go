package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/fulu/epoch_processing"
)

func TestMinimal_Fulu_EpochProcessing_SlashingsReset(t *testing.T) {
	epoch_processing.RunSlashingsResetTests(t, "minimal")
}
