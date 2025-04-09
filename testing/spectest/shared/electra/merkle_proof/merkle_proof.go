package merkle_proof

import (
	"testing"

	common "github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/common/merkle_proof"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/electra/ssz_static"
)

func RunMerkleProofTests(t *testing.T, config string) {
	common.RunMerkleProofTests(t, config, "electra", ssz_static.UnmarshalledSSZ)
}
