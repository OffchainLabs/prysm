package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/finality"
)

func TestMinimal_Gloas_Finality(t *testing.T) {
	t.Skip("gloas spec tests disabled until https://github.com/OffchainLabs/prysm/pull/16658")
	finality.RunFinalityTest(t, "minimal")
}
