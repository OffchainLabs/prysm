package operations

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/deneb/operations"
)

func TestMinimal_Deneb_Operations_Deposit(t *testing.T) {
	operations.RunDepositTest(t, "minimal")
}
