package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/capella/operations"
)

func TestMainnet_Capella_Operations_Attestation(t *testing.T) {
	operations.RunAttestationTest(t, "mainnet")
}
