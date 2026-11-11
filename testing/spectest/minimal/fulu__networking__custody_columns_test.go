package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/fulu/networking"
)

func TestMinimal_Fulu_Networking_CustodyGroups(t *testing.T) {
	networking.RunCustodyGroupsTest(t, "minimal", "fulu")
}

func TestMinimal_Fulu_Networking_ComputeCustodyColumnsForCustodyGroup(t *testing.T) {
	networking.RunComputeColumnsForCustodyGroupTest(t, "minimal", "fulu")
}
