package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/common/light_client"
)

func TestMainnet_Altair_LightClient_SingleMerkleProof(t *testing.T) {
	light_client.RunLightClientSingleMerkleProofTests(t, "minimal", version.Altair)
}
