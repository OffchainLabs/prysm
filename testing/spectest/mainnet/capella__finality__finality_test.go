package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/capella/finality"
)

func TestMainnet_Capella_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "mainnet")
}
