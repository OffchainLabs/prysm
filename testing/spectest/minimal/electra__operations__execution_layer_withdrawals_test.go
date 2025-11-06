package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/electra/operations"
)

func TestMinimal_Electra_Operations_WithdrawalRequest(t *testing.T) {
	operations.RunWithdrawalRequestTest(t, "minimal")
}
