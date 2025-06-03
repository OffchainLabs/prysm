package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/capella/fork"
)

func TestMainnet_Capella_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "mainnet")
}
