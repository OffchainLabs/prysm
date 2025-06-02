package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/electra/merkle_proof"
)

func TestMinimal_Electra_MerkleProof(t *testing.T) {
	merkle_proof.RunMerkleProofTests(t, "minimal")
}
