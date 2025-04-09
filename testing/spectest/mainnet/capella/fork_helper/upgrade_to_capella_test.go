package fork_helper

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/capella/fork"
)

func TestMainnet_Capella_UpgradeToCapella(t *testing.T) {
	fork.RunUpgradeToCapella(t, "mainnet")
}
