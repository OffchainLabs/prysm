package forkchoice

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/common/forkchoice"
)

func TestMinimal_Capella_Forkchoice(t *testing.T) {
	t.Skip("forkchoice changed in ePBS")
	forkchoice.Run(t, "minimal", version.Capella)
}
