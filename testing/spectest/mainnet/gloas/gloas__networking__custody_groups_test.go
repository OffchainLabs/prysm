package gloas

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/fulu/networking"
)

func TestMainnet_Gloas_Networking_CustodyGroups(t *testing.T) {
	networking.RunCustodyGroupsTest(t, "mainnet", "gloas")
}

func TestMainnet_Gloas_Networking_ComputeCustodyColumnsForCustodyGroup(t *testing.T) {
	networking.RunComputeColumnsForCustodyGroupTest(t, "mainnet", "gloas")
}
