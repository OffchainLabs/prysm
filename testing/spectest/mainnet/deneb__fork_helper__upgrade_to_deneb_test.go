package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/deneb/fork"
)

func TestMainnet_UpgradeToDeneb(t *testing.T) {
	fork.RunUpgradeToDeneb(t, "mainnet")
}
