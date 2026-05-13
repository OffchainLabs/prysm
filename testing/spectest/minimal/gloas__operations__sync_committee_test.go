package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/operations"
)

func TestMinimal_Gloas_Operations_SyncCommittee(t *testing.T) {
	operations.RunSyncCommitteeTest(t, "minimal")
}
