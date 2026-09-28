package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/forkchoice"
)

func TestMinimal_Altair_FastConfirmation(t *testing.T) {
	forkchoice.RunFastConfirmation(t, "minimal", version.Altair)
}
