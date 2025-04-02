package filesystem

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/spf13/afero"
)

type prunerScenario struct {
	name            string
	prunedBefore    primitives.Epoch
	retentionPeriod primitives.Epoch
	latest          primitives.Epoch
	expected        pruneExpectation
}

type pruneExpectation struct {
	called  bool
	arg     primitives.Epoch
	summary *pruneSummary
	err     error
}

func (e *pruneExpectation) record(before primitives.Epoch) (*pruneSummary, error) {
	e.called = true
	e.arg = before
	if e.summary == nil {
		e.summary = &pruneSummary{}
	}
	return e.summary, e.err
}

func TestPrunerNotify(t *testing.T) {
	defaultRetention := params.BeaconConfig().MinEpochsForBlobsSidecarsRequest
	cases := []prunerScenario{
		{
			name:            "last epoch of period",
			retentionPeriod: defaultRetention,
			prunedBefore:    11235,
			latest:          defaultRetention + 11235,
			expected:        pruneExpectation{called: false},
		},
		{
			name:            "within period",
			retentionPeriod: defaultRetention,
			prunedBefore:    11235,
			latest:          11235 + defaultRetention - 1,
			expected:        pruneExpectation{called: false},
		},
		{
			name:            "triggers",
			retentionPeriod: defaultRetention,
			prunedBefore:    11235,
			latest:          11235 + 1 + defaultRetention,
			expected:        pruneExpectation{called: true, arg: 11235 + 1},
		},
		{
			name:            "from zero - before first period",
			retentionPeriod: defaultRetention,
			prunedBefore:    0,
			latest:          defaultRetention - 1,
			expected:        pruneExpectation{called: false},
		},
		{
			name:            "from zero - at boundary",
			retentionPeriod: defaultRetention,
			prunedBefore:    0,
			latest:          defaultRetention,
			expected:        pruneExpectation{called: false},
		},
		{
			name:            "from zero - triggers",
			retentionPeriod: defaultRetention,
			prunedBefore:    0,
			latest:          defaultRetention + 1,
			expected:        pruneExpectation{called: true, arg: 1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := &pruneExpectation{}
			l := &mockLayout{pruneBeforeFunc: actual.record}
			pruner := &blobPruner{retentionPeriod: c.retentionPeriod}
			pruner.prunedBefore.Store(uint64(c.prunedBefore))
			done := pruner.notify(c.latest, l)
			<-done
			require.Equal(t, c.expected.called, actual.called)
			require.Equal(t, c.expected.arg, actual.arg)
		})
	}
}

func testSetupBlobIdentPaths(t *testing.T, fs afero.Fs, bs *BlobStorage, idents []testIdent) []blobIdent {
	created := make([]blobIdent, len(idents))
	for i, id := range idents {
		slot, err := slots.EpochStart(id.epoch)
		require.NoError(t, err)
		slot += id.offset
		_, scs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, slot, 1)
		sc := verification.FakeVerifyForTest(t, scs[0])
		require.NoError(t, bs.Save(sc))
		ident := identForSidecar(sc)
		_, err = fs.Stat(bs.layout.sszPath(ident))
		require.NoError(t, err)
		created[i] = ident
	}
	return created
}

func testAssertBlobsPruned(t *testing.T, fs afero.Fs, bs *BlobStorage, pruned, remain []blobIdent) {
	for _, id := range pruned {
		_, err := fs.Stat(bs.layout.sszPath(id))
		require.NotNil(t, err)
		require.Equal(t, true, os.IsNotExist(err))
	}
	for _, id := range remain {
		_, err := fs.Stat(bs.layout.sszPath(id))
		require.NoError(t, err)
	}
}

type testIdent struct {
	blobIdent
	offset primitives.Slot
}

func testRoots(n int) [][32]byte {
	roots := make([][32]byte, n)
	for i := range roots {
		binary.LittleEndian.PutUint32(roots[i][:], uint32(1+i))
	}
	return roots
}

func TestLayoutPruneBefore(t *testing.T) {
	roots := testRoots(10)
	cases := []struct {
		name        string
		pruned      []testIdent
		remain      []testIdent
		pruneBefore primitives.Epoch
		err         error
		sum         pruneSummary
	}{
		{
			name:        "none pruned",
			pruneBefore: 1,
			pruned:      []testIdent{},
			remain: []testIdent{
				{offset: 1, blobIdent: blobIdent{root: roots[0], epoch: 1, index: 0}},
				{offset: 1, blobIdent: blobIdent{root: roots[1], epoch: 1, index: 0}},
			},
		},
		{
			name:        "expected pruned before epoch",
			pruneBefore: 3,
			pruned: []testIdent{
				{offset: 0, blobIdent: blobIdent{root: roots[0], epoch: 1, index: 0}},
				{offset: 31, blobIdent: blobIdent{root: roots[1], epoch: 1, index: 5}},
				{offset: 0, blobIdent: blobIdent{root: roots[2], epoch: 2, index: 0}},
				{offset: 31, blobIdent: blobIdent{root: roots[3], epoch: 2, index: 3}},
			},
			remain: []testIdent{
				{offset: 0, blobIdent: blobIdent{root: roots[4], epoch: 3, index: 2}},  // boundary
				{offset: 31, blobIdent: blobIdent{root: roots[5], epoch: 3, index: 0}}, // boundary
				{offset: 0, blobIdent: blobIdent{root: roots[6], epoch: 4, index: 1}},
				{offset: 31, blobIdent: blobIdent{root: roots[7], epoch: 4, index: 5}},
			},
			sum: pruneSummary{blobsPruned: 4},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs, bs := NewEphemeralBlobStorageAndFs(t, WithLayout(LayoutNameByEpoch))
			pruned := testSetupBlobIdentPaths(t, fs, bs, c.pruned)
			remain := testSetupBlobIdentPaths(t, fs, bs, c.remain)
			sum, err := bs.layout.pruneBefore(c.pruneBefore)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			testAssertBlobsPruned(t, fs, bs, pruned, remain)
			require.Equal(t, c.sum.blobsPruned, sum.blobsPruned)
			require.Equal(t, len(c.pruned), sum.blobsPruned)
			require.Equal(t, len(c.sum.failedRemovals), len(sum.failedRemovals))
		})
	}
}
