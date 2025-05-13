package backfill

import (
	"io"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

type columnBisector struct {
	rootKeys     map[[32]byte]rootKey
	pidKeys      map[peer.ID]pidKey
	columnSource map[rootKey]map[uint64]pidKey
	bisected     map[pidKey][]blocks.RODataColumn
	pidIter      []peer.ID
	current      int
	next         int
	downscore    peerDownscorer
	errs         []error
	failures     map[rootKey]peerdas.ColumnIndices
}

type pidKey *peer.ID
type rootKey *[32]byte

var ErrColumnVerification = errors.New("column verification failed")

// TODO: write a method that iterates through the failed columns in the bisector and
// enables the retry code to retry all the failed columns.

func (c *columnBisector) addPeerColumns(pid peer.ID, columns ...blocks.RODataColumn) {
	pk := c.peerIdKey(pid)
	for _, col := range columns {
		c.setColumnSource(c.rootKey(col.BlockRoot()), col.Index, pk)
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

func (c *columnBisector) setColumnSource(rk rootKey, idx uint64, pk pidKey) {
	if c.columnSource == nil {
		c.columnSource = make(map[rootKey]map[uint64]pidKey)
	}
	if c.columnSource[rk] == nil {
		c.columnSource[rk] = make(map[uint64]pidKey)
	}
	c.columnSource[rk][idx] = pk
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

func (c *columnBisector) peerIdKey(pid peer.ID) pidKey {
	ptr, ok := c.pidKeys[pid]
	if ok {
		return ptr
	}
	c.pidKeys[pid] = &pid
	return c.pidKeys[pid]
}

func (c *columnBisector) peerFor(col blocks.RODataColumn) (pidKey, error) {
	r := c.columnSource[c.rootKey(col.BlockRoot())]
	if len(r) == 0 {
		return nil, errors.Wrap(das.ErrBisectInconsistent, "root not tracked")
	}
	if ptr, ok := r[col.Index]; ok {
		return ptr, nil
	}
	return nil, errors.Wrap(das.ErrBisectInconsistent, "index not tracked for root")
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
func (c *columnBisector) Bisect(columns []blocks.RODataColumn) error {
	for _, col := range columns {
		pid, err := c.peerFor(col)
		if err != nil {
			return errors.Wrap(err, "could not lookup peer for column")
		}
		c.bisected[pid] = append(c.bisected[pid], col)
	}
	c.pidIter = make([]peer.ID, 0, len(c.bisected))
	for pid := range c.bisected {
		c.pidIter = append(c.pidIter, *pid)
	}
	// The implementation of Next() assumes these are equal in
	// the base case.
	c.current, c.next = 0, 0
	return nil
}

// Next implements an iterator for the columnBisector.
// Each batch is from a single peer.
func (c *columnBisector) Next() ([]blocks.RODataColumn, error) {
	if c.next >= len(c.pidIter) {
		return nil, io.EOF
	}
	c.current = c.next
	pid := c.pidIter[c.current]
	cols := c.bisected[c.peerIdKey(pid)]
	c.next += 1
	return cols, nil
}

// Error implements das.Bisector.
func (c *columnBisector) Error() error {
	if len(c.errs) > 0 {
		return ErrColumnVerification
	}
	return nil
}

// OnError implements das.Bisector.
func (c *columnBisector) OnError(err error) {
	c.errs = append(c.errs, err)
	pid := c.pidIter[c.current]
	c.downscore(pid, "column verification error", err)
}

var _ das.Bisector = &columnBisector{}

func newColumnBisector(downscorer peerDownscorer) *columnBisector {
	return &columnBisector{
		rootKeys:     make(map[[32]byte]rootKey),
		pidKeys:      make(map[peer.ID]pidKey),
		columnSource: make(map[rootKey]map[uint64]pidKey),
		bisected:     make(map[pidKey][]blocks.RODataColumn),
		downscore:    downscorer,
	}
}
