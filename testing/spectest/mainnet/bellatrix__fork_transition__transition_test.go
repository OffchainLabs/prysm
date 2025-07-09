package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/bellatrix/fork"
)

func TestMainnet_Bellatrix_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "mainnet")
}
