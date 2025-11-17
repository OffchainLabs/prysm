package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/deneb/operations"
)

func TestMainnet_Deneb_Operations_Deposit(t *testing.T) {
	operations.RunDepositTest(t, "mainnet")
}
