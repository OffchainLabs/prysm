package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/operations"
)

func TestMainnet_Gloas_Operations_PayloadAttestation(t *testing.T) {
	operations.RunPayloadAttestationTest(t, "mainnet")
}
