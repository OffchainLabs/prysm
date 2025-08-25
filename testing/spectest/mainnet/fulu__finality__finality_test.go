package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/finality"
)

func TestMainnet_Fulu_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "mainnet")
}
