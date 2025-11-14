package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/deneb/operations"
)

func TestMinimal_Deneb_Operations_Attestation(t *testing.T) {
	operations.RunAttestationTest(t, "minimal")
}
