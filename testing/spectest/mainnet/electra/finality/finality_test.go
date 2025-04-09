package finality

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/electra/finality"
)

func TestMainnet_Electra_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "mainnet")
}
