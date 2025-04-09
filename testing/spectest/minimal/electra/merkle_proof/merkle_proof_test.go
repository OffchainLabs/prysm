package merkle_proof

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/electra/merkle_proof"
)

func TestMinimal_Electra_MerkleProof(t *testing.T) {
	merkle_proof.RunMerkleProofTests(t, "minimal")
}
