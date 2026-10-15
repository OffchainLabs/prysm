package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/operations"
)

func TestMinimal_Heze_Operations_PayloadAttestation(t *testing.T) {
	operations.RunPayloadAttestationTest(t, "minimal")
}
