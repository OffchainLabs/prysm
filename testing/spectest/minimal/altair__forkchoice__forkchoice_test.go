package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/forkchoice"
)

func TestMinimal_Altair_Forkchoice(t *testing.T) {
	t.Skip("Forkchoice tests can't pass because of backported changes from #4807")
	forkchoice.Run(t, "minimal", version.Altair)
}
