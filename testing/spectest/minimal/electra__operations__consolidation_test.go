package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/electra/operations"
)

func TestMinimal_Electra_Operations_Consolidation(t *testing.T) {
	operations.RunConsolidationTest(t, "minimal")
}
