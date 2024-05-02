package networking

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/eip7594/networking"
)

func TestMainnet_EIP7594_Networking_CustodyColumns(t *testing.T) {
	networking.RunCustodyColumnsTest(t, "mainnet")
}
