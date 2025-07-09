package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/operations"
)

func TestMainnet_Fulu_Operations_Deposit(t *testing.T) {
	operations.RunDepositTest(t, "mainnet")
}
