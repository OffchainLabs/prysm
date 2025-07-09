package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/altair/fork"
)

func TestMainnet_Altair_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "mainnet")
}
