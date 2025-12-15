package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/electra/finality"
)

func TestMinimal_Electra_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "minimal")
}
