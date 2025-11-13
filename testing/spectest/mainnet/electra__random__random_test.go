package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/electra/sanity"
)

func TestMainnet_Electra_Random(t *testing.T) {
	sanity.RunBlockProcessingTest(t, "mainnet", "random/random/pyspec_tests")
}
