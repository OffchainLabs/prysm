package backfill

import (
	"io"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

type columnBisector struct {
	rootKeys     map[[32]byte]rootKey
	columnSource map[rootKey]map[uint64]peer.ID
	bisected     map[peer.ID][]blocks.RODataColumn
	pidIter      []peer.ID
	current      int
	next         int
	downscore    peerDownscorer
	errs         []error
	failures     map[rootKey]peerdas.ColumnIndices
}

type rootKey *[32]byte

var errColumnVerification = errors.New("column verification failed")
var errBisectInconsistent = errors.New("state of bisector inconsistent with columns to bisect")

func (c *columnBisector) addPeerColumns(pid peer.ID, columns ...blocks.RODataColumn) {
	for _, col := range columns {
		c.setColumnSource(c.rootKey(col.BlockRoot()), col.Index, pid)
	}
}

// failuresFor returns the set of column indices that failed verification
// for the given block root.
func (c *columnBisector) failuresFor(root [32]byte) peerdas.ColumnIndices {
	return c.failures[c.rootKey(root)]
}

func (c *columnBisector) failingRoots() [][32]byte {
	roots := make([][32]byte, 0, len(c.failures))
	for rk := range c.failures {
		roots = append(roots, *rk)
	}
	return roots
}

func (c *columnBisector) setColumnSource(rk rootKey, idx uint64, pid peer.ID) {
	if c.columnSource == nil {
		c.columnSource = make(map[rootKey]map[uint64]peer.ID)
	}
	if c.columnSource[rk] == nil {
		c.columnSource[rk] = make(map[uint64]peer.ID)
	}
	c.columnSource[rk][idx] = pid
}

func (c *columnBisector) clearColumnSource(rk rootKey, idx uint64) {
	if c.columnSource == nil {
		return
	}
	if c.columnSource[rk] == nil {
		return
	}
	delete(c.columnSource[rk], idx)
	if len(c.columnSource[rk]) == 0 {
		delete(c.columnSource, rk)
	}
}

func (c *columnBisector) rootKey(root [32]byte) rootKey {
	ptr, ok := c.rootKeys[root]
	if ok {
		return ptr
	}
	c.rootKeys[root] = &root
	return c.rootKeys[root]
}

func (c *columnBisector) peerFor(col blocks.RODataColumn) (peer.ID, error) {
	r := c.columnSource[c.rootKey(col.BlockRoot())]
	if len(r) == 0 {
		return "", errors.Wrap(errBisectInconsistent, "root not tracked")
	}
	if pid, ok := r[col.Index]; ok {
		return pid, nil
	}
	return "", errors.Wrap(errBisectInconsistent, "index not tracked for root")
}

// reset prepares the columnBisector to be used to retry failed columns.
// it resets the peer sources of the failed columns and clears the failure records.
func (c *columnBisector) reset() {
	// reset all column sources for failed columns
	for rk, indices := range c.failures {
		for _, idx := range indices.ToSlice() {
			c.clearColumnSource(rk, idx)
		}
	}
	c.failures = make(map[rootKey]peerdas.ColumnIndices)
	c.errs = nil
}

// Bisect initializes columnBisector with the set of columns to bisect.
func (c *columnBisector) Bisect(columns []blocks.RODataColumn) (das.BisectionIterator, error) {
	for _, col := range columns {
		pid, err := c.peerFor(col)
		if err != nil {
			return nil, errors.Wrap(err, "could not lookup peer for column")
		}
		c.bisected[pid] = append(c.bisected[pid], col)
	}
	c.pidIter = make([]peer.ID, 0, len(c.bisected))
	for pid := range c.bisected {
		c.pidIter = append(c.pidIter, pid)
	}
	// The implementation of Next() assumes these are equal in
	// the base case.
	c.current, c.next = 0, 0
	return c, nil
}

// Next implements an iterator for the columnBisector.
// Each batch is from a single peer.
func (c *columnBisector) Next() ([]blocks.RODataColumn, error) {
	if c.next >= len(c.pidIter) {
		return nil, io.EOF
	}
	c.current = c.next
	pid := c.pidIter[c.current]
	cols := c.bisected[pid]
	c.next += 1
	return cols, nil
}

// Error implements das.Bisector.
func (c *columnBisector) Error() error {
	if len(c.errs) > 0 {
		return errColumnVerification
	}
	return nil
}

// OnError implements das.Bisector.
func (c *columnBisector) OnError(err error) {
	c.errs = append(c.errs, err)
	pid := c.pidIter[c.current]
	c.downscore(pid, "column verification error", err)

	// Track which roots failed by examining columns from the current peer
	columns := c.bisected[pid]
	for _, col := range columns {
		root := col.BlockRoot()
		rk := c.rootKey(root)
		if c.failures[rk] == nil {
			c.failures[rk] = make(peerdas.ColumnIndices)
		}
		c.failures[rk][col.Index] = struct{}{}
	}
}

var _ das.Bisector = &columnBisector{}
var _ das.BisectionIterator = &columnBisector{}

func newColumnBisector(downscorer peerDownscorer) *columnBisector {
	return &columnBisector{
		rootKeys:     make(map[[32]byte]rootKey),
		columnSource: make(map[rootKey]map[uint64]peer.ID),
		bisected:     make(map[peer.ID][]blocks.RODataColumn),
		failures:     make(map[rootKey]peerdas.ColumnIndices),
		downscore:    downscorer,
	}
}
