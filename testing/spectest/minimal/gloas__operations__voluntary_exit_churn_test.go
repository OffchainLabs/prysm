package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/operations"
)

func TestMinimal_Gloas_Operations_VoluntaryExitChurn(t *testing.T) {
	operations.RunVoluntaryExitChurnTest(t, "minimal")
}
