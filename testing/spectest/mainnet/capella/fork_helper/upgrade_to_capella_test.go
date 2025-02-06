package fork_helper

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v6/testing/spectest/shared/capella/fork"
)

func TestMainnet_Capella_UpgradeToCapella(t *testing.T) {
	fork.RunUpgradeToCapella(t, "mainnet")
}
