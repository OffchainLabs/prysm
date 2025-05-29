package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/operations"
)

func TestMinimal_Fulu_Operations_Consolidation(t *testing.T) {
	operations.RunConsolidationTest(t, "minimal")
}
