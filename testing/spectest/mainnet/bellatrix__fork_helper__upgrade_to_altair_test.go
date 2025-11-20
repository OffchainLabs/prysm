package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/bellatrix/fork"
)

func TestMainnet_Bellatrix_UpgradeToBellatrix(t *testing.T) {
	fork.RunUpgradeToBellatrix(t, "mainnet")
}
