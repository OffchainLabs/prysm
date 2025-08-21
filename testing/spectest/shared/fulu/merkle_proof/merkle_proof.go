package merkle_proof

import (
	"testing"

	common "github.com/OffchainLabs/prysm/v6/testing/spectest/shared/common/merkle_proof"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/ssz_static"
)

func RunMerkleProofTests(t *testing.T, config string) {
	common.RunMerkleProofTests(t, config, "fulu", ssz_static.UnmarshalledSSZ)
}
