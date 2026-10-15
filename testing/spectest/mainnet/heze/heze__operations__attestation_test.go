package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/operations"
)

func TestMainnet_Heze_Operations_Attestation(t *testing.T) {
	operations.RunAttestationTest(t, "mainnet")
}
