package backfill

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

var (
	errInvalidResponseOrder         = errors.New("out of order DataColumnSidecar response")
	errColumnResponseSlotOutOfRange = errors.New("slot out of range for DataColumnSidecar response")
	errColumnIndexNotRequested      = errors.New("index in DataColumnSidecar response not requested")
)

type columnBatch struct {
	first              primitives.Slot
	last               primitives.Slot
	custodyRequirement peerdas.ColumnIndices
	blockColumnsByRoot map[[32]byte]*blockColumns
	peerRank           *sync.ColumnPeerRank
}

type blockColumns struct {
	remaining   peerdas.ColumnIndices
	commitments [][]byte
}

func (cs *columnBatch) needed() peerdas.ColumnIndices {
	if len(cs.custodyRequirement) == 0 {
		return nil
	}
	search := peerdas.CopyTrueIndices(cs.custodyRequirement)
	ci := make(peerdas.ColumnIndices, len(search))
	// avoid iterating every single block+index by only searching for indices
	// we haven't found yet.
	for _, v := range cs.blockColumnsByRoot {
		if len(search) == 0 {
			return ci
		}
		for col := range search {
			if v.remaining[col] {
				ci[col] = true
				// We found the column, so we can delete it from the search.
				delete(search, col)
			}
		}
	}
	return ci
}

type columnSync struct {
	*columnBatch
	store   das.AvailabilityStore
	current primitives.Slot
}

func newColumnSync(b batch, blks verifiedROBlocks, current primitives.Slot, p p2p.P2P, vbs verifiedROBlocks, cfg *workerCfg) (*columnSync, error) {
	cb, err := buildColumnBatch(b, blks, p)
	if err != nil {
		return nil, err
	}
	if cb == nil {
		return &columnSync{}, nil
	}
	return &columnSync{
		columnBatch: cb,
		current:     current,
		store:       das.NewLazilyPersistentStoreColumn(cfg.cfs, p.NodeID(), cfg.ndcv, p.CustodyGroupCount()),
	}, nil
}

func (cs *columnSync) blockColumns(root [32]byte) *blockColumns {
	if cs.columnBatch == nil {
		return nil
	}
	return cs.columnBatch.blockColumnsByRoot[root]
}

func (cs *columnSync) columnsNeeded() peerdas.ColumnIndices {
	if cs.columnBatch == nil {
		return nil
	}
	return cs.columnBatch.needed()
}

func (cs *columnSync) request(reqCols []uint64) *ethpb.DataColumnSidecarsByRangeRequest {
	return sync.DataColumnSidecarsByRangeRequest(reqCols, cs.first, cs.last)
}

func (cs *columnSync) newValidatingColumnRequest(cols []uint64) *validatingColumnRequest {
	req := cs.request(cols)
	if req == nil {
		return nil
	}
	return &validatingColumnRequest{
		req:     req,
		columns: peerdas.ColumnIndicesFromSlice(cols),
		cs:      cs,
	}
}

type validatingColumnRequest struct {
	last    primitives.Slot
	req     *ethpb.DataColumnSidecarsByRangeRequest
	columns map[uint64]bool
	cs      *columnSync
}

func (v *validatingColumnRequest) validate(cd blocks.RODataColumn) error {
	return recordColumnSidecarDownload(cd, v.countedValidation(cd))
}

func recordColumnSidecarDownload(cd blocks.RODataColumn, valid bool) error {
	validity := "invalid"
	if valid {
		validity = "valid"
	}
	backfillDataColumnSidecarDownloaded.WithLabelValues(fmt.Sprintf("%d", cd.Index), validity).Inc()
	backfillBytesDataColumnSidecar.Add(float64(cd.SizeSSZ()))
	if !valid {
		return errors.New("invalid data column sidecar")
	}
	return nil
}

// When we call Persist we'll get the verification checks that are provided by the availability store.
// In addition to those checks this function calls rpcValidity which maintains a state machine across
// response values to ensure that the response is valid in the context of the overall request,
// like making sure that the block roots is one of the ones we expect based on the blocks we used to
// construct the request. It also does cheap sanity checks on the DataColumnSidecar values like
// ensuring that the commitments line up with the block.
func (v *validatingColumnRequest) countedValidation(cd blocks.RODataColumn) bool {
	if err := v.rpcValidity(cd); err != nil {
		log.WithError(err).WithField("slot", cd.Slot()).WithField("index", cd.Index).Error("invalid data column sidecar response")
		return false
	}
	root := cd.BlockRoot()
	expected := v.cs.blockColumns(root)
	if expected == nil {
		return false
	}
	// We don't need this column, but we trust the column state machine verified we asked for it as part of a range request.
	// So we can just skip over it and not try to persist it.
	if !expected.remaining[cd.Index] {
		return true
	}
	if len(cd.KzgCommitments) != len(expected.commitments) {
		log.WithField("slot", cd.Slot()).WithField("index", cd.Index).Error("unexpected number of commitments in data column sidecar")
		return false
	}
	for i, cmt := range cd.KzgCommitments {
		if !bytes.Equal(cmt, expected.commitments[i]) {
			log.WithField("slot", cd.Slot()).WithField("index", cd.Index).WithField("cmtIndex", i).Error("commitment in data column sidecar does not match expected commitment")
			return false
		}
	}
	if err := v.cs.store.Persist(v.cs.current, blocks.NewSidecarFromDataColumnSidecar(cd)); err != nil {
		log.WithError(err).Error("failed to persist data column")
		return false
	}
	delete(expected.remaining, cd.Index)
	return true
}

// rpcValidity checks that the individual DataColumnSidecar value is valid in the context of the overall response
// respecting the p2p spec rules for DataColumnSidecarByRange responses:
//   - values are in the requsted slot range
//   - values are in slot order
//   - block roots are canonical wrt the blocks we believe are canonical
//     (assuming previous block response from another peer was honest)
//   - there are not too many values in the response
//   - the column index is one of the requested columns
func (v *validatingColumnRequest) rpcValidity(col blocks.RODataColumn) error {
	slot := col.Slot()
	if v.last > slot {
		return errInvalidResponseOrder
	}
	if slot < v.req.StartSlot {
		return errors.Wrap(errColumnResponseSlotOutOfRange, "sidecar slot before request start")
	}
	if slot >= v.req.StartSlot+primitives.Slot(v.req.Count) {
		return errors.Wrap(errColumnResponseSlotOutOfRange, "sidecar slot after request end")
	}
	// This is an important check because we may have already satisfied this column for a given
	// block root but still requested it for the benefit of other blocks in the batch. So this check ensures
	// that it was part ofthe overall batch request.
	if !v.columns[col.Index] {
		return errColumnIndexNotRequested
	}
	v.last = col.Slot()
	return nil
}

func buildColumnBatch(b batch, fuluBlocks verifiedROBlocks, p p2p.P2P) (*columnBatch, error) {
	if len(fuluBlocks) == 0 {
		return nil, nil
	}

	fuluStart := params.BeaconConfig().FuluForkEpoch
	// If the batch end slot or last result block are pre-fulu, so are the rest.
	if slots.ToEpoch(b.end) < fuluStart || slots.ToEpoch(fuluBlocks[len(fuluBlocks)-1].Block().Slot()) < fuluStart {
		return nil, nil
	}
	// The last block in the batch is in fulu, but the first one is not.
	// Find the index of the first fulu block to exclude the pre-fulu blocks.
	if slots.ToEpoch(fuluBlocks[0].Block().Slot()) < fuluStart {
		fuluStart := sort.Search(len(fuluBlocks), func(i int) bool {
			return slots.ToEpoch(fuluBlocks[i].Block().Slot()) >= fuluStart
		})
		fuluBlocks = fuluBlocks[fuluStart:]
	}

	// Note that in the case where custody_group_count is the minimum CUSTODY_REQUIREMENT, we will
	// still download the extra columns dictated by SAMPLES_PER_SLOT. This is a hack to avoid complexity in the DA check.
	// We may want to revisit this to reduce bandwidth and storage for nodes with 0 validators attached.
	peerInfo, _, err := peerdas.Info(p.NodeID(), max(p.CustodyGroupCount(), params.BeaconConfig().SamplesPerSlot))
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}
	indices := peerdas.CopyTrueIndices(peerInfo.CustodyColumns)

	summary := &columnBatch{
		custodyRequirement: indices,
		blockColumnsByRoot: make(map[[32]byte]*blockColumns, len(fuluBlocks)),
	}
	for _, b := range fuluBlocks {
		cmts, err := b.Block().Body().BlobKzgCommitments()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get blob kzg commitments")
		}
		if len(cmts) == 0 {
			continue
		}
		slot := b.Block().Slot()
		if len(summary.blockColumnsByRoot) == 0 {
			summary.first = slot
		}
		summary.blockColumnsByRoot[b.Root()] = &blockColumns{
			remaining:   peerdas.CopyTrueIndices(indices),
			commitments: cmts,
		}
		summary.last = slot
	}

	return summary, nil
}
