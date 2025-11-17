package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/capella/operations"
)

func TestMinimal_Capella_Operations_Attestation(t *testing.T) {
	operations.RunAttestationTest(t, "minimal")
}
