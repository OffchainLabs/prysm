package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/forkchoice"
)

func TestMainnet_Fulu_Forkchoice(t *testing.T) {
	forkchoice.Run(t, "mainnet", version.Fulu)
}
