package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/operations"
)

func TestMainnet_Fulu_Operations_DepositRequests(t *testing.T) {
	operations.RunDepositRequestsTest(t, "mainnet")
}
