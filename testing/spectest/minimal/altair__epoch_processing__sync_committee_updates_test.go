package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/altair/epoch_processing"
)

func TestMinimal_Altair_EpochProcessing_SyncCommitteeUpdates(t *testing.T) {
	epoch_processing.RunSyncCommitteeUpdatesTests(t, "minimal")
}
