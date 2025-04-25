package fork_helper

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/electra/fork"
)

func TestMainnet_UpgradeToElectra(t *testing.T) {
	fork.RunUpgradeToElectra(t, "mainnet")
}
