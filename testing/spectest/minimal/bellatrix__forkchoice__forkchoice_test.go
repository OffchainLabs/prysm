package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/forkchoice"
)

func TestMinimal_Bellatrix_Forkchoice(t *testing.T) {
	forkchoice.Run(t, "minimal", version.Bellatrix)
}
