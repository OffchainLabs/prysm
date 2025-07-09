package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/electra/operations"
)

func TestMainnet_Electra_Operations_WithdrawalRequest(t *testing.T) {
	operations.RunWithdrawalRequestTest(t, "mainnet")
}
