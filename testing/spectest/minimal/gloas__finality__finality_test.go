package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/finality"
)

func TestMinimal_Gloas_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "minimal")
}
