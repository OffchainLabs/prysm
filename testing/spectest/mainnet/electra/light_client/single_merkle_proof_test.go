package light_client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/electra/light_client"
)

func TestMainnet_Electra_LightClient_SingleMerkleProof(t *testing.T) {
	light_client.RunLightClientSingleMerkleProofTests(t, "mainnet")
}
