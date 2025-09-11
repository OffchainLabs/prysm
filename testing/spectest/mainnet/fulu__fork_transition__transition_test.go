package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/fork"
)

func TestMainnet_Fulu_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "mainnet")
}
