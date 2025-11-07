package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/altair/operations"
)

func TestMinimal_Altair_Operations_SyncCommittee(t *testing.T) {
	operations.RunSyncCommitteeTest(t, "minimal")
}
