package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/forkchoice"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
)

func TestMainnet_Gloas_Forkchoice(t *testing.T) {
	utils.SkipGloasEip8148Divergence(t)
	forkchoice.Run(t, "mainnet", version.Gloas)
}
