package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/fulu/operations"
)

func TestMainnet_Fulu_Operations_Attestation(t *testing.T) {
	operations.RunAttestationTest(t, "mainnet")
}
