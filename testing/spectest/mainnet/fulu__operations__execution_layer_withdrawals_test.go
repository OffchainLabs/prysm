package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/operations"
)

func TestMainnet_Fulu_Operations_WithdrawalRequest(t *testing.T) {
	operations.RunWithdrawalRequestTest(t, "mainnet")
}
