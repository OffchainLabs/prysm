package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/operations"
)

func TestMainnet_Heze_Operations_VoluntaryExitChurn(t *testing.T) {
	operations.RunVoluntaryExitChurnTest(t, "mainnet")
}
