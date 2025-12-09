package backfill

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

const testBlobGenBlobCount = 3

func testBlobGen(t *testing.T, start primitives.Slot, n int) ([]blocks.ROBlock, [][]blocks.ROBlob) {
	blks := make([]blocks.ROBlock, n)
	blobs := make([][]blocks.ROBlob, n)
	for i := range n {
		bk, bl := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, start+primitives.Slot(i), testBlobGenBlobCount)
		blks[i] = bk
		blobs[i] = bl
	}
	return blks, blobs
}

func setupCurrentNeeds(t *testing.T, current primitives.Slot) das.SyncNeeds {
	cs := func() primitives.Slot { return current }
	sn, err := das.NewSyncNeeds(cs, nil, 0)
	require.NoError(t, err)
	return sn
}

func TestValidateNext_happy(t *testing.T) {
	startSlot := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	current := startSlot + 65
	blks, blobs := testBlobGen(t, startSlot, 4)
	cfg := &blobSyncConfig{
		nbv:          testNewBlobVerifier(),
		store:        filesystem.NewEphemeralBlobStorage(t),
		currentNeeds: mockCurrentNeedsFunc(0, current+1),
	}
	//expected :=
	expected, err := verifiedROBlocks(blks).blobIdents(cfg.currentNeeds)
	require.NoError(t, err)
	require.Equal(t, len(blks)*testBlobGenBlobCount, len(expected))
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
	denebSlot, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	require.NoError(t, err)
	current := primitives.Slot(128)
	syncNeeds := setupCurrentNeeds(t, current)
	cfg := &blobSyncConfig{
		nbv:          testNewBlobVerifier(),
		store:        filesystem.NewEphemeralBlobStorage(t),
		currentNeeds: syncNeeds.Currently,
	}
	blks, blobs := testBlobGen(t, denebSlot, 2)
	bsync, err := newBlobSync(current, blks, cfg)
	require.NoError(t, err)
	require.ErrorIs(t, bsync.validateNext(blobs[len(blobs)-1][0]), errUnexpectedResponseContent)
}

func TestValidateNext_sigMatch(t *testing.T) {
	denebSlot, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	require.NoError(t, err)
	current := primitives.Slot(128)
	syncNeeds := setupCurrentNeeds(t, current)
	cfg := &blobSyncConfig{
		nbv:          testNewBlobVerifier(),
		store:        filesystem.NewEphemeralBlobStorage(t),
		currentNeeds: syncNeeds.Currently,
	}
	blks, blobs := testBlobGen(t, denebSlot, 1)
	bsync, err := newBlobSync(current, blks, cfg)
	require.NoError(t, err)
	blobs[0][0].SignedBlockHeader.Signature = bytesutil.PadTo([]byte("derp"), 48)
	require.ErrorIs(t, bsync.validateNext(blobs[0][0]), verification.ErrInvalidProposerSignature)
}

func TestValidateNext_errorsFromVerifier(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	current := primitives.Slot(ds + 96)
	blks, blobs := testBlobGen(t, ds+31, 1)

	cn := mockCurrentNeedsFunc(0, current+1)
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
				nbv:          testNewBlobVerifier(c.cb),
				store:        filesystem.NewEphemeralBlobStorage(t),
				currentNeeds: cn,
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
