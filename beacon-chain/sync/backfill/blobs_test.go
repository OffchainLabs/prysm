package backfill

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func testBlobGen(t *testing.T, start primitives.Slot, n int) ([]blocks.ROBlock, [][]blocks.ROBlob) {
	blks := make([]blocks.ROBlock, n)
	blobs := make([][]blocks.ROBlob, n)
	for i := 0; i < n; i++ {
		bk, bl := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, start+primitives.Slot(i), 3)
		blks[i] = bk
		blobs[i] = bl
	}
	return blks, blobs
}

func TestValidateNext_happy(t *testing.T) {
	current := primitives.Slot(128)
	blks, blobs := testBlobGen(t, 63, 4)
	cfg := &blobSyncConfig{
		retentionStart: 0,
		nbv:            testNewBlobVerifier(),
		store:          filesystem.NewEphemeralBlobStorage(t),
	}
	bsync, err := newBlobSync(current, blks, cfg)
	require.NoError(t, err)
	nb := 0
	for i := range blobs {
		bs := blobs[i]
		for ib := range bs {
			require.NoError(t, bsync.validateNext(bs[ib]))
			nb += 1
		}
	}
	require.Equal(t, nb, bsync.next)
	// we should get an error if we read another blob.
	require.ErrorIs(t, bsync.validateNext(blobs[0][0]), errUnexpectedResponseSize)
}

func TestValidateNext_cheapErrors(t *testing.T) {
	current := primitives.Slot(128)
	blks, blobs := testBlobGen(t, 63, 2)
	cfg := &blobSyncConfig{
		retentionStart: 0,
		nbv:            testNewBlobVerifier(),
		store:          filesystem.NewEphemeralBlobStorage(t),
	}
	bsync, err := newBlobSync(current, blks, cfg)
	require.NoError(t, err)
	require.ErrorIs(t, bsync.validateNext(blobs[len(blobs)-1][0]), errUnexpectedResponseContent)
}

func TestValidateNext_sigMatch(t *testing.T) {
	current := primitives.Slot(128)
	blks, blobs := testBlobGen(t, 63, 1)
	cfg := &blobSyncConfig{
		retentionStart: 0,
		nbv:            testNewBlobVerifier(),
		store:          filesystem.NewEphemeralBlobStorage(t),
	}
	bsync, err := newBlobSync(current, blks, cfg)
	require.NoError(t, err)
	blobs[0][0].SignedBlockHeader.Signature = bytesutil.PadTo([]byte("derp"), 48)
	require.ErrorIs(t, bsync.validateNext(blobs[0][0]), verification.ErrInvalidProposerSignature)
}

func TestValidateNext_errorsFromVerifier(t *testing.T) {
	current := primitives.Slot(128)
	blks, blobs := testBlobGen(t, 63, 1)
	cases := []struct {
		name string
		err  error
		cb   func(*verification.MockBlobVerifier)
	}{
		{
			name: "index oob",
			err:  verification.ErrBlobIndexInvalid,
			cb: func(v *verification.MockBlobVerifier) {
				v.ErrBlobIndexInBounds = verification.ErrBlobIndexInvalid
			},
		},
		{
			name: "not inclusion proven",
			err:  verification.ErrSidecarInclusionProofInvalid,
			cb: func(v *verification.MockBlobVerifier) {
				v.ErrSidecarInclusionProven = verification.ErrSidecarInclusionProofInvalid
			},
		},
		{
			name: "not kzg proof valid",
			err:  verification.ErrSidecarKzgProofInvalid,
			cb: func(v *verification.MockBlobVerifier) {
				v.ErrSidecarKzgProofVerified = verification.ErrSidecarKzgProofInvalid
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &blobSyncConfig{
				retentionStart: 0,
				nbv:            testNewBlobVerifier(c.cb),
				store:          filesystem.NewEphemeralBlobStorage(t),
			}
			bsync, err := newBlobSync(current, blks, cfg)
			require.NoError(t, err)
			require.ErrorIs(t, bsync.validateNext(blobs[0][0]), c.err)
		})
	}
}

func testNewBlobVerifier(opts ...func(*verification.MockBlobVerifier)) verification.NewBlobVerifier {
	return func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
		v := &verification.MockBlobVerifier{}
		for i := range opts {
			opts[i](v)
		}
		return v
	}
}
