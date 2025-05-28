package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/common/forkchoice"
)

func TestMainnet_Fulu_Forkchoice(t *testing.T) {
	forkchoice.Run(t, "mainnet", version.Fulu)
}
