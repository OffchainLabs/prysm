package validator

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestServer_buildBlobSidecars(t *testing.T) {
	kzgCommitments := [][]byte{bytesutil.PadTo([]byte{'a'}, 48), bytesutil.PadTo([]byte{'b'}, 48)}
	blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockDeneb())
	require.NoError(t, err)
	require.NoError(t, blk.SetBlobKzgCommitments(kzgCommitments))
	proof, err := hexutil.Decode("0xb4021b0de10f743893d4f71e1bf830c019e832958efd6795baf2f83b8699a9eccc5dc99015d8d4d8ec370d0cc333c06a")
	require.NoError(t, err)
	scs, err := BuildBlobSidecars(blk, [][]byte{
		make([]byte, fieldparams.BlobLength), make([]byte, fieldparams.BlobLength),
	}, [][]byte{
		proof, proof,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(scs))

	inclusionProof0, err := blocks.MerkleProofKZGCommitment(blk.Block().Body(), 0)
	require.NoError(t, err)
	require.DeepEqual(t, inclusionProof0, scs[0].CommitmentInclusionProof)

	inclusionProof1, err := blocks.MerkleProofKZGCommitment(blk.Block().Body(), 1)
	require.NoError(t, err)
	require.DeepEqual(t, inclusionProof1, scs[1].CommitmentInclusionProof)
}
