package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/operations"
)

func TestMinimal_Gloas_Operations_Attestation(t *testing.T) {
	operations.RunAttestationTest(t, "minimal")
}
