package verify

import (
	"fmt"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestBlobAlignsWithBlock(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	tests := []struct {
		name         string
		blockAndBlob func(t *testing.T) (blocks.ROBlock, []blocks.ROBlob)
		err          error
	}{
		{
			name: "happy path",
			blockAndBlob: func(t *testing.T) (blocks.ROBlock, []blocks.ROBlob) {
				return util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, 1)
			},
		},
		{
			name: "mismatched roots",
			blockAndBlob: func(t *testing.T) (blocks.ROBlock, []blocks.ROBlob) {
				blk, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, 1)
				tweaked := blobs[0].BlobSidecar
				tweaked.SignedBlockHeader.Header.Slot = tweaked.SignedBlockHeader.Header.Slot + 1
				tampered, err := blocks.NewROBlob(tweaked)
				require.NoError(t, err)
				return blk, []blocks.ROBlob{tampered}
			},
			err: ErrBlobBlockMisaligned,
		},
		{
			name: "mismatched roots - fake",
			blockAndBlob: func(t *testing.T) (blocks.ROBlock, []blocks.ROBlob) {
				blk, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, 1)
				copied := blobs[0].BlobSidecar
				// exact same header, mess with the root
				fake, err := blocks.NewROBlobWithRoot(copied, bytesutil.ToBytes32([]byte("derp")))
				require.NoError(t, err)
				return blk, []blocks.ROBlob{fake}
			},
			err: ErrBlobBlockMisaligned,
		},
		{
			name: "before deneb",
			blockAndBlob: func(t *testing.T) (blocks.ROBlock, []blocks.ROBlob) {
				cb := util.NewBeaconBlockCapella()
				blk, err := blocks.NewSignedBeaconBlock(cb)
				require.NoError(t, err)
				rob, err := blocks.NewROBlock(blk)
				require.NoError(t, err)
				return rob, []blocks.ROBlob{{}}
			},
		},
	}

	for _, tt := range tests {
		block, blobs := tt.blockAndBlob(t)
		for i := range blobs {
			t.Run(tt.name+fmt.Sprintf(" blob %d", i), func(t *testing.T) {
				err := BlobAlignsWithBlock(blobs[i], block)
				if tt.err == nil {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, tt.err)
				}
			})
		}
	}
}

func TestBlobAlignsWithBlock_OOBIndexReturnsError(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	nCommitments := 3

	roBlock, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, nCommitments)
	blockRoot := roBlock.Root()

	// Sanity: valid blob (index=0) works fine.
	require.NoError(t, BlobAlignsWithBlock(blobs[0], roBlock))

	// OOB index: 3 is past the 3 commitments but within MaxBlobsPerBlock (6 for Deneb).
	maxBlobs := params.BeaconConfig().MaxBlobsPerBlock(ds)
	oobIndex := uint64(nCommitments)
	require.Equal(t, true, oobIndex < uint64(maxBlobs),
		"precondition: OOB index must be < MaxBlobsPerBlock for this test")

	oobSidecar := &ethpb.BlobSidecar{
		SignedBlockHeader:        blobs[0].SignedBlockHeader,
		Index:                    oobIndex,
		Blob:                     make([]byte, fieldparams.BlobSize),
		KzgCommitment:            make([]byte, 48),
		KzgProof:                 make([]byte, 48),
		CommitmentInclusionProof: blobs[0].CommitmentInclusionProof,
	}
	oobBlob, err := blocks.NewROBlobWithRoot(oobSidecar, blockRoot)
	require.NoError(t, err)

	// Must return ErrIncorrectBlobIndex, not panic.
	err = BlobAlignsWithBlock(oobBlob, roBlock)
	require.ErrorIs(t, err, ErrIncorrectBlobIndex)
}

func TestBlobAlignsWithBlock_MaxIndexEdge(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	nCommitments := 3

	roBlock, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, nCommitments)
	blockRoot := roBlock.Root()

	// index = nCommitments-1 (last valid): must not error.
	require.NoError(t, BlobAlignsWithBlock(blobs[nCommitments-1], roBlock))

	// index = nCommitments (first OOB): must return ErrIncorrectBlobIndex.
	oobSidecar := &ethpb.BlobSidecar{
		SignedBlockHeader:        blobs[0].SignedBlockHeader,
		Index:                    uint64(nCommitments),
		Blob:                     make([]byte, fieldparams.BlobSize),
		KzgCommitment:            make([]byte, 48),
		KzgProof:                 make([]byte, 48),
		CommitmentInclusionProof: blobs[0].CommitmentInclusionProof,
	}
	oobBlob, err := blocks.NewROBlobWithRoot(oobSidecar, blockRoot)
	require.NoError(t, err)

	err = BlobAlignsWithBlock(oobBlob, roBlock)
	require.ErrorIs(t, err, ErrIncorrectBlobIndex)
}

func TestBlobAlignsWithBlock_AllValidIndicesSucceed(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	nCommitments := 4

	roBlock, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, nCommitments)

	for i := 0; i < nCommitments; i++ {
		i := i
		t.Run(fmt.Sprintf("index_%d", i), func(t *testing.T) {
			require.NoError(t, BlobAlignsWithBlock(blobs[i], roBlock))
		})
	}
}
