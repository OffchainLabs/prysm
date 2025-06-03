package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/merkle_proof"
)

func TestMinimal_Fulu_MerkleProof(t *testing.T) {
	merkle_proof.RunMerkleProofTests(t, "minimal")
}
