package networking

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/networking"
)

func TestMainnet_Fulu_Networking_CustodyColumns(t *testing.T) {
	networking.RunCustodyColumnsTest(t, "minimal")
}
