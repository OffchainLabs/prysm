package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/fulu/operations"
)

func TestMinimal_Fulu_Operations_BlockHeader(t *testing.T) {
	operations.RunBlockHeaderTest(t, "minimal")
}
