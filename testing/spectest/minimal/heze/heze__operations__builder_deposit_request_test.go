package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/operations"
)

func TestMinimal_Heze_Operations_BuilderDepositRequest(t *testing.T) {
	operations.RunBuilderDepositRequestTest(t, "minimal")
}
