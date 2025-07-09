package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/electra/finality"
)

func TestMainnet_Electra_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "mainnet")
}
