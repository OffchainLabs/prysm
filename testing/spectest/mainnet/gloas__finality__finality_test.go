package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/finality"
)

func TestMainnet_Gloas_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "mainnet")
}
