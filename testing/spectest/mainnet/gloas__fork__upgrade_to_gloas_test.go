package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/fork"
)

func TestMainnet_UpgradeToGloas(t *testing.T) {
	fork.RunUpgradeToGloas(t, "mainnet")
}
