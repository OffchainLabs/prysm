package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/common/forkchoice"
)

func TestMainnet_Deneb_Forkchoice(t *testing.T) {
	t.Skip("forkchoice changed in ePBS")
	forkchoice.Run(t, "mainnet", version.Deneb)
}
