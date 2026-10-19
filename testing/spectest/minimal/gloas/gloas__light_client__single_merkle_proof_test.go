package gloas

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/light_client"
)

func TestMinimal_Gloas_LightClient_SingleMerkleProof(t *testing.T) {
	light_client.RunLightClientSingleMerkleProofTests(t, "minimal", version.Gloas)
}
