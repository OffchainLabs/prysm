package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/epoch_processing"
)

func TestMinimal_Gloas_EpochProcessing_ProposerLookahead(t *testing.T) {
	epoch_processing.RunProposerLookaheadTests(t, "minimal")
}
