package backfill

import (
	"io"
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

// mockDownscorer is a simple downscorer that tracks calls
type mockDownscorer struct {
	calls []struct {
		pid peer.ID
		msg string
		err error
	}
}

func (m *mockDownscorer) downscoreCall(pid peer.ID, msg string, err error) {
	m.calls = append(m.calls, struct {
		pid peer.ID
		msg string
		err error
	}{pid, msg, err})
}

// createTestDataColumn creates a test data column with the given parameters.
// nBlobs determines the number of cells, commitments, and proofs.
func createTestDataColumn(t *testing.T, root [32]byte, index uint64, nBlobs int) util.DataColumnParam {
	commitments := make([][]byte, nBlobs)
	cells := make([][]byte, nBlobs)
	proofs := make([][]byte, nBlobs)

	for i := range nBlobs {
		commitments[i] = make([]byte, 48)
		cells[i] = make([]byte, 0)
		proofs[i] = make([]byte, 48)
	}

	return util.DataColumnParam{
		Index:          index,
		Column:         cells,
		KzgCommitments: commitments,
		KzgProofs:      proofs,
		Slot:           primitives.Slot(1),
		BodyRoot:       root[:],
		StateRoot:      make([]byte, 32),
		ParentRoot:     make([]byte, 32),
	}
}

// createTestPeerID creates a test peer ID from a string seed.
func createTestPeerID(t *testing.T, seed string) peer.ID {
	pid, err := peer.Decode(seed)
	require.NoError(t, err)
	return pid
}

// TestNewColumnBisector verifies basic initialization
func TestNewColumnBisector(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	require.NotNil(t, cb)
	require.NotNil(t, cb.rootKeys)
	require.NotNil(t, cb.columnSource)
	require.NotNil(t, cb.bisected)
	require.Equal(t, 0, cb.current)
	require.Equal(t, 0, cb.next)
}

// TestAddAndIterateColumns demonstrates creating test columns and iterating
func TestAddAndIterateColumns(t *testing.T) {
	root := [32]byte{1, 0, 0}
	params := []util.DataColumnParam{
		createTestDataColumn(t, root, 0, 2),
		createTestDataColumn(t, root, 1, 2),
	}

	roColumns, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params)
	require.Equal(t, 2, len(roColumns))

	// Create downscorer and bisector
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	// Create test peer ID
	pid1 := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")

	// Add columns from peer
	cb.addPeerColumns(pid1, roColumns...)

	// Bisect and verify iteration
	iter, err := cb.Bisect(roColumns)
	require.NoError(t, err)
	require.NotNil(t, iter)

	// Get first (and only) batch from the peer
	batch, err := iter.Next()
	require.NoError(t, err)
	require.Equal(t, 2, len(batch))

	// Next should return EOF
	_, err = iter.Next()
	require.Equal(t, io.EOF, err)
}

// TestRootKeyDeduplication verifies that rootKey returns the same pointer for identical roots
func TestRootKeyDeduplication(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root := [32]byte{1, 2, 3}
	key1 := cb.rootKey(root)
	key2 := cb.rootKey(root)

	// Should be the same pointer
	require.Equal(t, key1, key2)
}

// TestMultipleRootsAndPeers verifies handling of multiple distinct roots and peer IDs
func TestMultipleRootsAndPeers(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root1 := [32]byte{1, 0, 0}
	root2 := [32]byte{2, 0, 0}
	root3 := [32]byte{3, 0, 0}

	pid1 := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")
	pid2 := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMr")

	// Register multiple columns with different roots and peers
	params1 := createTestDataColumn(t, root1, 0, 2)
	params2 := createTestDataColumn(t, root2, 1, 2)
	params3 := createTestDataColumn(t, root3, 2, 2)

	cols1, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{params1})
	cols2, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{params2})
	cols3, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{params3})

	cb.addPeerColumns(pid1, cols1...)
	cb.addPeerColumns(pid2, cols2...)
	cb.addPeerColumns(pid1, cols3...)

	// Verify roots and peers are tracked
	require.Equal(t, 3, len(cb.rootKeys))
}

// TestSetColumnSource verifies that columns from different peers are properly tracked
func TestSetColumnSource(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	// Create multiple peers with columns
	root1 := [32]byte{1, 0, 0}
	root2 := [32]byte{2, 0, 0}
	root3 := [32]byte{3, 0, 0}

	pid1 := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")
	pid2 := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMr")

	// Create columns for peer1: 2 columns
	params1 := []util.DataColumnParam{
		createTestDataColumn(t, root1, 0, 1),
		createTestDataColumn(t, root2, 1, 1),
	}
	// Create columns for peer2: 2 columns
	params2 := []util.DataColumnParam{
		createTestDataColumn(t, root3, 0, 1),
		createTestDataColumn(t, root1, 2, 1),
	}

	cols1, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params1)
	cols2, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params2)

	// Register columns from both peers
	cb.addPeerColumns(pid1, cols1...)
	cb.addPeerColumns(pid2, cols2...)

	// Use Bisect to verify columns are grouped by peer
	allCols := append(cols1, cols2...)
	iter, err := cb.Bisect(allCols)
	require.NoError(t, err)

	// Sort the pidIter so batch order is deterministic
	slices.Sort(cb.pidIter)

	// Collect all batches (order is non-deterministic due to map iteration)
	var batches [][]blocks.RODataColumn
	for {
		batch, err := iter.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		batches = append(batches, batch)
	}

	// Verify we got exactly 2 batches
	require.Equal(t, 2, len(batches))

	// Find which batch is from which peer by examining the columns
	pid1Batch := map[peer.ID][]blocks.RODataColumn{pid1: nil, pid2: nil}
	for _, batch := range batches {
		if len(batch) == 0 {
			continue
		}
		// All columns in a batch are from the same peer
		col := batch[0]
		colPeer, err := cb.peerFor(col)
		require.NoError(t, err)
		// Compare dereferenced peer.ID values rather than pointers
		if colPeer == pid1 {
			pid1Batch[pid1] = batch
		} else if colPeer == pid2 {
			pid1Batch[pid2] = batch
		}
	}

	// Verify peer1's batch has 2 columns
	require.NotNil(t, pid1Batch[pid1])
	require.Equal(t, 2, len(pid1Batch[pid1]))
	for _, col := range pid1Batch[pid1] {
		colPeer, err := cb.peerFor(col)
		require.NoError(t, err)
		require.Equal(t, pid1, colPeer)
	}

	// Verify peer2's batch has 2 columns
	require.NotNil(t, pid1Batch[pid2])
	require.Equal(t, 2, len(pid1Batch[pid2]))
	for _, col := range pid1Batch[pid2] {
		colPeer, err := cb.peerFor(col)
		require.NoError(t, err)
		require.Equal(t, pid2, colPeer)
	}
}

// TestClearColumnSource verifies column removal and cleanup of empty maps
func TestClearColumnSource(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root := [32]byte{1, 0, 0}
	rk := cb.rootKey(root)
	pid := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")

	cb.setColumnSource(rk, 0, pid)
	cb.setColumnSource(rk, 1, pid)
	require.Equal(t, 2, len(cb.columnSource[rk]))

	// Clear one column
	cb.clearColumnSource(rk, 0)
	require.Equal(t, 1, len(cb.columnSource[rk]))

	// Clear the last column - should remove the root entry
	cb.clearColumnSource(rk, 1)
	_, exists := cb.columnSource[rk]
	require.Equal(t, false, exists)
}

// TestClearNonexistentColumn ensures clearing non-existent columns doesn't crash
func TestClearNonexistentColumn(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root := [32]byte{1, 0, 0}
	rk := cb.rootKey(root)

	// Should not panic
	cb.clearColumnSource(rk, 99)
}

// TestFailuresFor verifies failuresFor returns correct failures for a block root
func TestFailuresFor(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root := [32]byte{1, 0, 0}
	rk := cb.rootKey(root)

	// Initially no failures
	failures := cb.failuresFor(root)
	require.Equal(t, 0, len(failures.ToSlice()))

	// Set some failures
	cb.failures[rk] = peerdas.ColumnIndices{0: struct{}{}, 1: struct{}{}, 2: struct{}{}}
	failures = cb.failuresFor(root)
	require.Equal(t, 3, len(failures.ToSlice()))
}

// TestFailingRoots ensures failingRoots returns all roots with failures
func TestFailingRoots(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root1 := [32]byte{1, 0, 0}
	root2 := [32]byte{2, 0, 0}
	rk1 := cb.rootKey(root1)
	rk2 := cb.rootKey(root2)

	cb.failures[rk1] = peerdas.ColumnIndices{0: struct{}{}}
	cb.failures[rk2] = peerdas.ColumnIndices{1: struct{}{}}

	failingRoots := cb.failingRoots()
	require.Equal(t, 2, len(failingRoots))
}

// TestPeerFor verifies peerFor correctly returns the peer for a column
func TestPeerFor(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root := [32]byte{1, 0, 0}
	pid := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")

	params := createTestDataColumn(t, root, 0, 2)
	cols, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{params})

	// Use addPeerColumns to properly register the column
	cb.addPeerColumns(pid, cols[0])

	peerKey, err := cb.peerFor(cols[0])
	require.NoError(t, err)
	require.NotNil(t, peerKey)
}

// TestPeerForNotTracked ensures error when root not tracked
func TestPeerForNotTracked(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root := [32]byte{1, 0, 0}
	params := createTestDataColumn(t, root, 0, 2)
	cols, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{params})

	// Don't add any columns - root is not tracked
	_, err := cb.peerFor(cols[0])
	require.ErrorIs(t, err, errBisectInconsistent)
}

// TestBisectGroupsByMultiplePeers ensures columns grouped by their peer source
func TestBisectGroupsByMultiplePeers(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root := [32]byte{1, 0, 0}
	pid1 := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")
	pid2 := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMr")

	params1 := createTestDataColumn(t, root, 0, 2)
	params2 := createTestDataColumn(t, root, 1, 2)

	cols1, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{params1})
	cols2, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{params2})

	cb.addPeerColumns(pid1, cols1...)
	cb.addPeerColumns(pid2, cols2...)

	// Bisect both columns
	iter, err := cb.Bisect(append(cols1, cols2...))
	require.NoError(t, err)

	// Sort the pidIter so that batch order is deterministic
	slices.Sort(cb.pidIter)

	// Should get two separate batches, one from each peer
	batch1, err := iter.Next()
	require.NoError(t, err)
	require.Equal(t, 1, len(batch1))

	batch2, err := iter.Next()
	require.NoError(t, err)
	require.Equal(t, 1, len(batch2))

	_, err = iter.Next()
	require.Equal(t, io.EOF, err)
}

// TestOnError verifies OnError records errors and calls downscorer
func TestOnError(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	pid := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")
	cb.pidIter = append(cb.pidIter, pid)
	cb.current = 0

	testErr := errors.New("test error")
	cb.OnError(testErr)

	require.Equal(t, 1, len(cb.errs))
	require.Equal(t, 1, len(downscorer.calls))
	require.Equal(t, pid, downscorer.calls[0].pid)
}

// TestErrorReturnAfterOnError ensures Error() returns non-nil after OnError called
func TestErrorReturnAfterOnError(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	pid := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")
	cb.pidIter = append(cb.pidIter, pid)
	cb.current = 0

	require.NoError(t, cb.Error())

	cb.OnError(errors.New("test error"))
	require.NotNil(t, cb.Error())
}

// TestResetClearsFailures verifies reset clears all failures and errors
func TestResetClearsFailures(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root := [32]byte{1, 0, 0}
	rk := cb.rootKey(root)

	cb.failures[rk] = peerdas.ColumnIndices{0: struct{}{}, 1: struct{}{}}
	cb.errs = []error{errors.New("test")}

	cb.reset()

	require.Equal(t, 0, len(cb.failures))
	require.Equal(t, 0, len(cb.errs))
}

// TestResetClearsColumnSources ensures reset clears column sources for failed columns
func TestResetClearsColumnSources(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root := [32]byte{1, 0, 0}
	rk := cb.rootKey(root)
	pid := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")

	cb.setColumnSource(rk, 0, pid)
	cb.setColumnSource(rk, 1, pid)

	cb.failures[rk] = peerdas.ColumnIndices{0: struct{}{}, 1: struct{}{}}

	cb.reset()

	// Column sources for the failed root should be cleared
	_, exists := cb.columnSource[rk]
	require.Equal(t, false, exists)
}

// TestBisectResetBisectAgain tests end-to-end multiple bisect cycles with reset
func TestBisectResetBisectAgain(t *testing.T) {
	downscorer := &mockDownscorer{}
	root := [32]byte{1, 0, 0}
	pid := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")

	params := createTestDataColumn(t, root, 0, 2)
	cols, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{params})

	// First bisect with fresh bisector
	cb1 := newColumnBisector(downscorer.downscoreCall)
	cb1.addPeerColumns(pid, cols...)
	iter, err := cb1.Bisect(cols)
	require.NoError(t, err)

	batch, err := iter.Next()
	require.NoError(t, err)
	require.Equal(t, 1, len(batch))

	_, err = iter.Next()
	require.Equal(t, io.EOF, err)

	// Second bisect with a new bisector (simulating retry with reset)
	cb2 := newColumnBisector(downscorer.downscoreCall)
	cb2.addPeerColumns(pid, cols...)
	iter, err = cb2.Bisect(cols)
	require.NoError(t, err)

	batch, err = iter.Next()
	require.NoError(t, err)
	require.Equal(t, 1, len(batch))
}

// TestBisectEmptyColumns tests Bisect with empty column list
func TestBisectEmptyColumns(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	var emptyColumns []util.DataColumnParam
	roColumns, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, emptyColumns)

	iter, err := cb.Bisect(roColumns)
	// This should not error with empty columns
	if err == nil {
		_, err := iter.Next()
		require.Equal(t, io.EOF, err)
	}
}

// TestCompleteFailureFlow tests marking a peer as failed and tracking failure roots
func TestCompleteFailureFlow(t *testing.T) {
	downscorer := &mockDownscorer{}
	cb := newColumnBisector(downscorer.downscoreCall)

	root1 := [32]byte{1, 0, 0}
	root2 := [32]byte{2, 0, 0}
	root3 := [32]byte{3, 0, 0}

	pid1 := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMq")
	pid2 := createTestPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTSc34pP8r3hidQPQMr")

	// Create columns: pid1 provides columns for root1 and root2, pid2 provides for root3
	params1 := []util.DataColumnParam{
		createTestDataColumn(t, root1, 0, 2),
		createTestDataColumn(t, root2, 1, 2),
	}
	params2 := []util.DataColumnParam{
		createTestDataColumn(t, root3, 2, 2),
	}

	cols1, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params1)
	cols2, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params2)

	cb.addPeerColumns(pid1, cols1...)
	cb.addPeerColumns(pid2, cols2...)

	allCols := append(cols1, cols2...)
	iter, err := cb.Bisect(allCols)
	require.NoError(t, err)

	// sort the pidIter so that the test is deterministic
	slices.Sort(cb.pidIter)

	batch1, err := iter.Next()
	require.NoError(t, err)
	require.Equal(t, 2, len(batch1))

	// Determine which peer the first batch came from
	firstBatchPeer := batch1[0]
	colPeer, err := cb.peerFor(firstBatchPeer)
	require.NoError(t, err)
	expectedPeer := colPeer

	// Extract the roots from batch1 to ensure we can track them
	rootsInBatch1 := make(map[[32]byte]bool)
	for _, col := range batch1 {
		rootsInBatch1[col.BlockRoot()] = true
	}

	// Mark the first batch's peer as failed
	cb.OnError(errors.New("peer verification failed"))

	// Verify downscorer was called for the peer that had the first batch
	require.Equal(t, 1, len(downscorer.calls))
	require.Equal(t, expectedPeer, downscorer.calls[0].pid)

	// Verify that failures contains the roots from batch1
	require.Equal(t, len(rootsInBatch1), len(cb.failingRoots()))

	// Get remaining batches until EOF
	batch2, err := iter.Next()
	require.NoError(t, err)
	require.Equal(t, 1, len(batch2))

	_, err = iter.Next()
	require.Equal(t, io.EOF, err)

	// Verify failingRoots matches the roots from the failed batch
	failingRoots := cb.failingRoots()
	require.Equal(t, len(rootsInBatch1), len(failingRoots))

	// Verify the failing roots are exactly the ones from batch1
	failingRootsMap := make(map[[32]byte]bool)
	for _, root := range failingRoots {
		failingRootsMap[root] = true
	}
	for root := range rootsInBatch1 {
		require.Equal(t, true, failingRootsMap[root])
	}
}
