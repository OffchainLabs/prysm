package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/common/forkchoice"
)

func TestMinimal_Electra_Forkchoice(t *testing.T) {
	forkchoice.Run(t, "minimal", version.Electra)
}
