package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/operations"
)

func TestMinimal_Gloas_Operations_Deposit(t *testing.T) {
	operations.RunDepositTest(t, "minimal")
}
