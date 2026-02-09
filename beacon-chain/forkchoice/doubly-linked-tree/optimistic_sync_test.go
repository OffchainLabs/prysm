package doublylinkedtree

import (
	"sort"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// We test the algorithm to update a node from SYNCING to INVALID
// We start with the same diagram as above:
//
//	              E -- F
//	             /
//	       C -- D
//	      /      \
//	A -- B        G -- H -- I
//	      \        \
//	       J        -- K -- L
//
// And every block in the Fork choice is optimistic.
func TestPruneInvalid(t *testing.T) {
	tests := []struct {
		name             string
		root             [32]byte // the root of the new INVALID block
		parentRoot       [32]byte // the root of the parent block
		parentHash       [32]byte // the execution hash of the parent block
		lastValidHash    [32]byte // the last valid execution hash
		wantedNodeNumber int
		wantedRoots      [][32]byte
		wantedErr        error
	}{
		{ // Bogus LVH, root not in forkchoice
			name: "bogus LVH not in forkchoice",
			root: [32]byte{'x'}, parentRoot: [32]byte{'i'}, parentHash: [32]byte{'I'}, lastValidHash: [32]byte{'R'},
			wantedNodeNumber: 13, wantedRoots: [][32]byte{},
		},
		{ // Bogus LVH
			name: "bogus LVH",
			root: [32]byte{'i'}, parentRoot: [32]byte{'h'}, parentHash: [32]byte{'H'}, lastValidHash: [32]byte{'R'},
			wantedNodeNumber: 13, wantedRoots: [][32]byte{},
		},
		{
			name: "wanted j",
			root: [32]byte{'j'}, parentRoot: [32]byte{'b'}, parentHash: [32]byte{'B'}, lastValidHash: [32]byte{'B'},
			wantedNodeNumber: 13, wantedRoots: [][32]byte{},
		},
		{
			name: "wanted 5",
			root: [32]byte{'c'}, parentRoot: [32]byte{'b'}, parentHash: [32]byte{'B'}, lastValidHash: [32]byte{'B'},
			wantedNodeNumber: 5,
			wantedRoots: [][32]byte{
				{'f'},
				{'e'},
				{'i'},
				{'h'},
				{'l'},
				{'k'},
				{'g'},
				{'d'},
			},
		},
		{
			name: "wanted i",
			root: [32]byte{'i'}, parentRoot: [32]byte{'h'}, parentHash: [32]byte{'H'}, lastValidHash: [32]byte{'H'},
			wantedNodeNumber: 13, wantedRoots: [][32]byte{},
		},
		{
			name: "wanted i and h",
			root: [32]byte{'h'}, parentRoot: [32]byte{'g'}, parentHash: [32]byte{'G'}, lastValidHash: [32]byte{'G'},
			wantedNodeNumber: 12, wantedRoots: [][32]byte{{'i'}},
		},
		{
			name: "wanted i--g",
			root: [32]byte{'g'}, parentRoot: [32]byte{'d'}, parentHash: [32]byte{'D'}, lastValidHash: [32]byte{'D'},
			wantedNodeNumber: 9, wantedRoots: [][32]byte{{'i'}, {'h'}, {'l'}, {'k'}},
		},
		{
			name: "wanted 9",
			root: [32]byte{'i'}, parentRoot: [32]byte{'h'}, parentHash: [32]byte{'H'}, lastValidHash: [32]byte{'D'},
			wantedNodeNumber: 9, wantedRoots: [][32]byte{{'i'}, {'h'}, {'l'}, {'k'}},
		},
		{
			name: "wanted f and e",
			root: [32]byte{'f'}, parentRoot: [32]byte{'e'}, parentHash: [32]byte{'E'}, lastValidHash: [32]byte{'D'},
			wantedNodeNumber: 12, wantedRoots: [][32]byte{{'f'}},
		},
		{
			name: "wanted 6",
			root: [32]byte{'h'}, parentRoot: [32]byte{'g'}, parentHash: [32]byte{'G'}, lastValidHash: [32]byte{'C'},
			wantedNodeNumber: 6,
			wantedRoots: [][32]byte{
				{'f'}, {'e'}, {'i'}, {'h'}, {'l'}, {'k'}, {'g'},
			},
		},
		{
			name: "wanted 9 again",
			root: [32]byte{'g'}, parentRoot: [32]byte{'d'}, parentHash: [32]byte{'D'}, lastValidHash: [32]byte{'E'},
			wantedNodeNumber: 9, wantedRoots: [][32]byte{{'i'}, {'h'}, {'l'}, {'k'}},
		},
		{
			name: "wanted 13",
			root: [32]byte{'z'}, parentRoot: [32]byte{'j'}, parentHash: [32]byte{'J'}, lastValidHash: [32]byte{'B'},
			wantedNodeNumber: 13, wantedRoots: [][32]byte{},
		},
		{
			name: "wanted empty",
			root: [32]byte{'z'}, parentRoot: [32]byte{'j'}, parentHash: [32]byte{'J'}, lastValidHash: [32]byte{'J'},
			wantedNodeNumber: 13, wantedRoots: [][32]byte{},
		},
		{
			name: "errInvalidParentRoot",
			root: [32]byte{'j'}, parentRoot: [32]byte{'a'}, parentHash: [32]byte{'A'}, lastValidHash: [32]byte{'B'},
			wantedErr: errInvalidParentRoot,
		},
		{
			name: "root z",
			root: [32]byte{'z'}, parentRoot: [32]byte{'h'}, parentHash: [32]byte{'H'}, lastValidHash: [32]byte{'D'},
			wantedNodeNumber: 9, wantedRoots: [][32]byte{{'i'}, {'h'}, {'l'}, {'k'}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			f := setup(1, 1)
			require.NoError(t, f.SetOptimisticToValid(ctx, [32]byte{}))
			state, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'j'}, [32]byte{'b'}, [32]byte{'J'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 103, [32]byte{'d'}, [32]byte{'c'}, [32]byte{'D'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 104, [32]byte{'e'}, [32]byte{'d'}, [32]byte{'E'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 104, [32]byte{'g'}, [32]byte{'d'}, [32]byte{'G'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'f'}, [32]byte{'e'}, [32]byte{'F'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'h'}, [32]byte{'g'}, [32]byte{'H'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'k'}, [32]byte{'g'}, [32]byte{'K'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 106, [32]byte{'i'}, [32]byte{'h'}, [32]byte{'I'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))
			state, blkRoot, err = prepareForkchoiceState(ctx, 106, [32]byte{'l'}, [32]byte{'k'}, [32]byte{'L'}, 1, 1)
			require.NoError(t, err)
			require.NoError(t, f.InsertNode(ctx, state, blkRoot))

			roots, err := f.store.setOptimisticToInvalid(t.Context(), tc.root, tc.parentRoot, tc.parentHash, tc.lastValidHash)
			if tc.wantedErr == nil {
				require.NoError(t, err)
				require.Equal(t, len(tc.wantedRoots), len(roots))
				require.DeepEqual(t, tc.wantedRoots, roots)
				require.Equal(t, tc.wantedNodeNumber, f.NodeCount())
			} else {
				require.ErrorIs(t, tc.wantedErr, err)
			}
		})
	}
}

// This is a regression test (10445)
func TestSetOptimisticToInvalid_ProposerBoost(t *testing.T) {
	ctx := t.Context()
	f := setup(1, 1)

	state, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	f.store.proposerBoostRoot = [32]byte{'c'}
	f.store.previousProposerBoostScore = 10
	f.store.previousProposerBoostRoot = [32]byte{'b'}

	_, err = f.SetOptimisticToInvalid(ctx, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'B'}, [32]byte{'A'})
	require.NoError(t, err)
	require.Equal(t, uint64(0), f.store.previousProposerBoostScore)
	require.DeepEqual(t, [32]byte{}, f.store.proposerBoostRoot)
	require.DeepEqual(t, params.BeaconConfig().ZeroHash, f.store.previousProposerBoostRoot)
}

// This is a regression test (10565)
//     ----- C
//   /
//  A <- B
//   \
//     ----------D
// D is invalid

func TestSetOptimisticToInvalid_CorrectChildren(t *testing.T) {
	ctx := t.Context()
	f := setup(1, 1)

	state, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'c'}, [32]byte{'a'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 103, [32]byte{'d'}, [32]byte{'a'}, [32]byte{'D'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	_, err = f.store.setOptimisticToInvalid(ctx, [32]byte{'d'}, [32]byte{'a'}, [32]byte{'A'}, [32]byte{'A'})
	require.NoError(t, err)
	require.Equal(t, 2, len(f.store.fullNodeByRoot[[32]byte{'a'}].children))
}

// Pow       |      Pos
//
//	CA -- A -- B -- C-----D
//	 \          \--------------E
//	  \
//	   ----------------------F -- G
//
// B is INVALID
func TestSetOptimisticToInvalid_ForkAtMerge(t *testing.T) {
	ctx := t.Context()
	f := setup(1, 1)

	st, root, err := prepareForkchoiceState(ctx, 100, [32]byte{'r'}, [32]byte{}, [32]byte{}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 101, [32]byte{'a'}, [32]byte{'r'}, [32]byte{}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 102, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 103, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 104, [32]byte{'d'}, [32]byte{'c'}, [32]byte{'D'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 105, [32]byte{'e'}, [32]byte{'b'}, [32]byte{'E'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 106, [32]byte{'f'}, [32]byte{'r'}, [32]byte{'F'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 107, [32]byte{'g'}, [32]byte{'f'}, [32]byte{'G'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	roots, err := f.SetOptimisticToInvalid(ctx, [32]byte{'x'}, [32]byte{'d'}, [32]byte{'D'}, [32]byte{})
	require.NoError(t, err)
	require.Equal(t, 4, len(roots))
	sort.Slice(roots, func(i, j int) bool {
		return bytesutil.BytesToUint64BigEndian(roots[i][:]) < bytesutil.BytesToUint64BigEndian(roots[j][:])
	})
	require.DeepEqual(t, roots, [][32]byte{{'b'}, {'c'}, {'d'}, {'e'}})
}

// Pow       |      Pos
//
//	CA -------- B -- C-----D
//	 \           \--------------E
//	  \
//	   --A -------------------------F -- G
//
// B is INVALID
func TestSetOptimisticToInvalid_ForkAtMerge_bis(t *testing.T) {
	ctx := t.Context()
	f := setup(1, 1)

	st, root, err := prepareForkchoiceState(ctx, 100, [32]byte{'r'}, [32]byte{}, [32]byte{}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 101, [32]byte{'a'}, [32]byte{'r'}, [32]byte{}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 102, [32]byte{'b'}, [32]byte{}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 103, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 104, [32]byte{'d'}, [32]byte{'c'}, [32]byte{'D'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 105, [32]byte{'e'}, [32]byte{'b'}, [32]byte{'E'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 106, [32]byte{'f'}, [32]byte{'a'}, [32]byte{'F'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 107, [32]byte{'g'}, [32]byte{'f'}, [32]byte{'G'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	roots, err := f.SetOptimisticToInvalid(ctx, [32]byte{'x'}, [32]byte{'d'}, [32]byte{'D'}, [32]byte{})
	require.NoError(t, err)
	require.Equal(t, 4, len(roots))
	sort.Slice(roots, func(i, j int) bool {
		return bytesutil.BytesToUint64BigEndian(roots[i][:]) < bytesutil.BytesToUint64BigEndian(roots[j][:])
	})
	require.DeepEqual(t, roots, [][32]byte{{'b'}, {'c'}, {'d'}, {'e'}})
}

func TestSetOptimisticToValid(t *testing.T) {
	f := setup(1, 1)
	op, err := f.IsOptimistic([32]byte{})
	require.NoError(t, err)
	require.Equal(t, true, op)
	require.NoError(t, f.SetOptimisticToValid(t.Context(), [32]byte{}))
	op, err = f.IsOptimistic([32]byte{})
	require.NoError(t, err)
	require.Equal(t, false, op)
}
