package gloas

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/fulu/networking"
)

func TestMinimal_Gloas_Networking_CustodyGroups(t *testing.T) {
	networking.RunCustodyGroupsTest(t, "minimal", "gloas")
}

func TestMinimal_Gloas_Networking_ComputeCustodyColumnsForCustodyGroup(t *testing.T) {
	networking.RunComputeColumnsForCustodyGroupTest(t, "minimal", "gloas")
}
