package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/capella/epoch_processing"
)

func TestMinimal_Capella_EpochProcessing_JustificationAndFinalization(t *testing.T) {
	epoch_processing.RunJustificationAndFinalizationTests(t, "minimal")
}
