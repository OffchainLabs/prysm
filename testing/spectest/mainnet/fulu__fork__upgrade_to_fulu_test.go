package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/fulu/fork"
)

func TestMainnet_UpgradeToFulu(t *testing.T) {
	fork.RunUpgradeToFulu(t, "mainnet")
}
