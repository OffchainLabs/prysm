package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/deneb/merkle_proof"
)

func TestMainnet_Deneb_MerkleProof(t *testing.T) {
	merkle_proof.RunMerkleProofTests(t, "mainnet")
}
