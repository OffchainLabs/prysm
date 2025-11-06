package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/fulu/finality"
)

func TestMinimal_Fulu_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "minimal")
}
