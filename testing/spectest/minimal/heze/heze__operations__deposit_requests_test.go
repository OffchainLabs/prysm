package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/operations"
)

func TestMinimal_Heze_Operations_DepositRequests(t *testing.T) {
	operations.RunDepositRequestsTest(t, "minimal")
}
