package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/bellatrix/finality"
)

func TestMinimal_Bellatrix_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "minimal")
}
