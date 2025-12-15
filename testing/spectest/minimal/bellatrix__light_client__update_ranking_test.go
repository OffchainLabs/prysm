package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/light_client"
)

func TestMainnet_Bellatrix_LightClient_UpdateRanking(t *testing.T) {
	light_client.RunLightClientUpdateRankingTests(t, "minimal", version.Bellatrix)
}
