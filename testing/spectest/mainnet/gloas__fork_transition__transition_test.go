package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/fork"
)

func TestMainnet_Gloas_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "mainnet")
}
