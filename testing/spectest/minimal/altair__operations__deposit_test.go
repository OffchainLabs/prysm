package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/altair/operations"
)

func TestMinimal_Altair_Operations_Deposit(t *testing.T) {
	operations.RunDepositTest(t, "minimal")
}
