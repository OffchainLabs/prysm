package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/altair/operations"
)

func TestMinimal_Altair_Operations_AttesterSlashing(t *testing.T) {
	operations.RunAttesterSlashingTest(t, "minimal")
}
