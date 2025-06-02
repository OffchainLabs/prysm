package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/networking"
)

func TestMainnet_Fulu_Networking_CustodyGroups(t *testing.T) {
	networking.RunCustodyGroupsTest(t, "mainnet")
}

func TestMainnet_Fulu_Networking_ComputeCustodyColumnsForCustodyGroup(t *testing.T) {
	networking.RunComputeColumnsForCustodyGroupTest(t, "mainnet")
}
