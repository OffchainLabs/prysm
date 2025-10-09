package das

import (
	"bytes"
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	errors "github.com/pkg/errors"
)

func Test_commitmentsToCheck(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2
	fulu := primitives.Slot(params.BeaconConfig().FuluForkEpoch) * params.BeaconConfig().SlotsPerEpoch
	windowSlots, err := slots.EpochEnd(params.BeaconConfig().MinEpochsForBlobsSidecarsRequest)
	require.NoError(t, err)
	windowSlots = windowSlots + primitives.Slot(params.BeaconConfig().FuluForkEpoch)
	maxBlobs := params.LastNetworkScheduleEntry().MaxBlobsPerBlock
	commits := make([][]byte, maxBlobs+1)
	for i := 0; i < len(commits); i++ {
		commits[i] = bytesutil.PadTo([]byte{byte(i)}, 48)
	}
	cases := []struct {
		name    string
		commits [][]byte
		block   func(*testing.T) blocks.ROBlock
		slot    primitives.Slot
		err     error
	}{
		{
			name: "pre deneb",
			block: func(t *testing.T) blocks.ROBlock {
				bb := util.NewBeaconBlockBellatrix()
				sb, err := blocks.NewSignedBeaconBlock(bb)
				require.NoError(t, err)
				rb, err := blocks.NewROBlock(sb)
				require.NoError(t, err)
				return rb
			},
		},
		{
			name: "commitments within da",
			block: func(t *testing.T) blocks.ROBlock {
				d := util.NewBeaconBlockFulu()
				d.Block.Body.BlobKzgCommitments = commits[:maxBlobs]
				d.Block.Slot = fulu + 100
				sb, err := blocks.NewSignedBeaconBlock(d)
				require.NoError(t, err)
				rb, err := blocks.NewROBlock(sb)
				require.NoError(t, err)
				return rb
			},
			commits: commits[:maxBlobs],
			slot:    fulu + 100,
		},
		{
			name: "commitments outside da",
			block: func(t *testing.T) blocks.ROBlock {
				d := util.NewBeaconBlockFulu()
				d.Block.Slot = fulu
				// block is from slot 0, "current slot" is window size +1 (so outside the window)
				d.Block.Body.BlobKzgCommitments = commits[:maxBlobs]
				sb, err := blocks.NewSignedBeaconBlock(d)
				require.NoError(t, err)
				rb, err := blocks.NewROBlock(sb)
				require.NoError(t, err)
				return rb
			},
			slot: fulu + windowSlots + 1,
		},
		{
			name: "excessive commitments",
			block: func(t *testing.T) blocks.ROBlock {
				d := util.NewBeaconBlockFulu()
				d.Block.Slot = fulu + 100
				// block is from slot 0, "current slot" is window size +1 (so outside the window)
				d.Block.Body.BlobKzgCommitments = commits
				sb, err := blocks.NewSignedBeaconBlock(d)
				require.NoError(t, err)
				rb, err := blocks.NewROBlock(sb)
				require.NoError(t, err)
				c, err := rb.Block().Body().BlobKzgCommitments()
				require.NoError(t, err)
				require.Equal(t, true, len(c) > params.BeaconConfig().MaxBlobsPerBlock(sb.Block().Slot()))
				return rb
			},
			slot: windowSlots + 1,
			err:  errIndexOutOfBounds,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := c.block(t)
			co, err := commitmentsToCheck(b, c.slot)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, len(c.commits), len(co))
			for i := 0; i < len(c.commits); i++ {
				require.Equal(t, true, bytes.Equal(c.commits[i], co[i]))
			}
		})
	}
}

func TestLazilyPersistent_Missing(t *testing.T) {
	ctx := t.Context()
	store := filesystem.NewEphemeralBlobStorage(t)
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)

	blk, blobSidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, 3)

	mbv := &mockBlobBatchVerifier{t: t, scs: blobSidecars}
	as := NewLazilyPersistentStore(store, mbv)

	// Only one commitment persisted, should return error with other indices
	require.NoError(t, as.Persist(ds, blobSidecars[2]))
	err := as.IsDataAvailable(ctx, ds, blk)
	require.ErrorIs(t, err, errMissingSidecar)

	// All but one persisted, return missing idx
	require.NoError(t, as.Persist(ds, blobSidecars[0]))
	err = as.IsDataAvailable(ctx, ds, blk)
	require.ErrorIs(t, err, errMissingSidecar)

	// All persisted, return nil
	require.NoError(t, as.Persist(ds, blobSidecars...))

	require.NoError(t, as.IsDataAvailable(ctx, ds, blk))
}

func TestLazilyPersistent_Mismatch(t *testing.T) {
	ctx := t.Context()
	store := filesystem.NewEphemeralBlobStorage(t)
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)

	blk, blobSidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, 3)

	mbv := &mockBlobBatchVerifier{t: t, err: errors.New("kzg check should not run")}
	blobSidecars[0].KzgCommitment = bytesutil.PadTo([]byte("nope"), 48)
	as := NewLazilyPersistentStore(store, mbv)

	// Only one commitment persisted, should return error with other indices
	require.NoError(t, as.Persist(ds, blobSidecars[0]))
	err := as.IsDataAvailable(ctx, ds, blk)
	require.NotNil(t, err)
	require.ErrorIs(t, err, errCommitmentMismatch)
}

func TestLazyPersistOnceCommitted(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	_, blobSidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, 6)

	as := NewLazilyPersistentStore(filesystem.NewEphemeralBlobStorage(t), &mockBlobBatchVerifier{})
	// stashes as expected
	require.NoError(t, as.Persist(ds, blobSidecars...))
	// ignores duplicates
	require.ErrorIs(t, as.Persist(ds, blobSidecars...), ErrDuplicateSidecar)

	// ignores index out of bound
	blobSidecars[0].Index = 6
	require.ErrorIs(t, as.Persist(ds, blobSidecars[0]), errIndexOutOfBounds)
	_, moreBlobSidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, 4)

	// ignores sidecars before the retention period
	slotOOB := util.SlotAtEpoch(t, params.BeaconConfig().MinEpochsForBlobsSidecarsRequest)
	slotOOB += ds + 32
	require.NoError(t, as.Persist(slotOOB, moreBlobSidecars[0]))

	// doesn't ignore new sidecars with a different block root
	require.NoError(t, as.Persist(ds, moreBlobSidecars...))
}

type mockBlobBatchVerifier struct {
	t        *testing.T
	scs      []blocks.ROBlob
	err      error
	verified map[[32]byte]primitives.Slot
}

var _ BlobBatchVerifier = &mockBlobBatchVerifier{}

func (m *mockBlobBatchVerifier) VerifiedROBlobs(_ context.Context, _ blocks.ROBlock, scs []blocks.ROBlob) ([]blocks.VerifiedROBlob, error) {
	require.Equal(m.t, len(scs), len(m.scs))
	for i := range m.scs {
		require.Equal(m.t, m.scs[i], scs[i])
	}
	vscs := verification.FakeVerifySliceForTest(m.t, scs)
	return vscs, m.err
}

func (m *mockBlobBatchVerifier) MarkVerified(root [32]byte, slot primitives.Slot) {
	if m.verified == nil {
		m.verified = make(map[[32]byte]primitives.Slot)
	}
	m.verified[root] = slot
}
