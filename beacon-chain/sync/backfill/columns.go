package backfill

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

var (
	errInvalidDataColumnResponse = errors.New("invalid DataColumnSidecar response")
	errUnexpectedBlockRoot       = errors.Wrap(errInvalidDataColumnResponse, "unexpected sidecar block root")
	errCommitmentLengthMismatch  = errors.Wrap(errInvalidDataColumnResponse, "sidecar has different commitment count than block")
	errCommitmentValueMismatch   = errors.Wrap(errInvalidDataColumnResponse, "sidecar commitments do not match block")
)

type columnBatch struct {
	first         primitives.Slot
	last          primitives.Slot
	custodyGroups peerdas.ColumnIndices
	toDownload    map[[32]byte]*toDownload
}

type toDownload struct {
	remaining   peerdas.ColumnIndices
	commitments [][]byte
}

func (cs *columnBatch) needed() peerdas.ColumnIndices {
	// make a copy that we can modify to reduce search iterations.
	search := cs.custodyGroups.ToMap()
	ci := peerdas.ColumnIndices{}
	for _, v := range cs.toDownload {
		if len(search) == 0 {
			return ci
		}
		for col := range search {
			if v.remaining.Has(col) {
				ci.Set(col)
				// avoid iterating every single block+index by only searching for indices
				// we haven't found yet.
				delete(search, col)
			}
		}
	}
	return ci
}

type columnSync struct {
	*columnBatch
	store    *das.LazilyPersistentStoreColumn
	current  primitives.Slot
	peer     peer.ID
	bisector *columnBisector
}

func newColumnSync(ctx context.Context, b batch, blks verifiedROBlocks, current primitives.Slot, p p2p.P2P, vbs verifiedROBlocks, cfg *workerCfg) (*columnSync, error) {
	cgc, err := p.CustodyGroupCount(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "custody group count")
	}
	cb, err := buildColumnBatch(ctx, b, blks, p)
	if err != nil {
		return nil, err
	}
	if cb == nil {
		return &columnSync{}, nil
	}

	bisector := newColumnBisector(cfg.downscore)
	return &columnSync{
		columnBatch: cb,
		current:     current,
		store:       das.NewLazilyPersistentStoreColumn(cfg.colStore, cfg.newVC, p.NodeID(), cgc, bisector),
		bisector:    bisector,
	}, nil
}

func (cs *columnSync) blockColumns(root [32]byte) *toDownload {
	if cs.columnBatch == nil {
		return nil
	}
	return cs.columnBatch.toDownload[root]
}

func (cs *columnSync) columnsNeeded() peerdas.ColumnIndices {
	if cs.columnBatch == nil {
		return peerdas.ColumnIndices{}
	}
	return cs.columnBatch.needed()
}

func (cs *columnSync) request(reqCols []uint64) *ethpb.DataColumnSidecarsByRangeRequest {
	return sync.DataColumnSidecarsByRangeRequest(reqCols, cs.first, cs.last)
}

type validatingColumnRequest struct {
	req        *ethpb.DataColumnSidecarsByRangeRequest
	columnSync *columnSync
	bisector   *columnBisector
}

func (v *validatingColumnRequest) validate(cd blocks.RODataColumn) (err error) {
	defer func(validity string, start time.Time) {
		dataColumnSidecarVerifyMs.Observe(float64(time.Since(start).Milliseconds()))
		if err != nil {
			validity = "invalid"
		}
		dataColumnSidecarDownloadCount.WithLabelValues(fmt.Sprintf("%d", cd.Index), validity).Inc()
		dataColumnSidecarDownloadBytes.Add(float64(cd.SizeSSZ()))
	}("valid", time.Now())
	return v.countedValidation(cd)
}

// When we call Persist we'll get the verification checks that are provided by the availability store.
// In addition to those checks this function calls rpcValidity which maintains a state machine across
// response values to ensure that the response is valid in the context of the overall request,
// like making sure that the block roots is one of the ones we expect based on the blocks we used to
// construct the request. It also does cheap sanity checks on the DataColumnSidecar values like
// ensuring that the commitments line up with the block.
func (v *validatingColumnRequest) countedValidation(cd blocks.RODataColumn) error {
	root := cd.BlockRoot()
	expected := v.columnSync.blockColumns(root)
	if expected == nil {
		return errors.Wrapf(errUnexpectedBlockRoot, "root=%#x, slot=%d", root, cd.Slot())
	}
	// We don't need this column, but we trust the column state machine verified we asked for it as part of a range request.
	// So we can just skip over it and not try to persist it.
	if !expected.remaining.Has(cd.Index) {
		return nil
	}
	if len(cd.KzgCommitments) != len(expected.commitments) {
		return errors.Wrapf(errCommitmentLengthMismatch, "root=%#x, slot=%d, index=%d", root, cd.Slot(), cd.Index)
	}
	for i, cmt := range cd.KzgCommitments {
		if !bytes.Equal(cmt, expected.commitments[i]) {
			return errors.Wrapf(errCommitmentValueMismatch, "root=%#x, slot=%d, index=%d", root, cd.Slot(), cd.Index)
		}
	}
	if err := v.columnSync.store.Persist(v.columnSync.current, cd); err != nil {
		return errors.Wrap(err, "persisting data column")
	}
	v.bisector.addPeerColumns(v.columnSync.peer, cd)
	expected.remaining.Unset(cd.Index)
	return nil
}

func currentCustodiedColumns(ctx context.Context, p p2p.P2P) (peerdas.ColumnIndices, error) {
	cgc, err := p.CustodyGroupCount(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "custody group count")
	}

	// Note that in the case where custody_group_count is the minimum CUSTODY_REQUIREMENT, we will
	// still download the extra columns dictated by SAMPLES_PER_SLOT. This is a hack to avoid complexity in the DA check.
	// We may want to revisit this to reduce bandwidth and storage for nodes with 0 validators attached.
	peerInfo, _, err := peerdas.Info(p.NodeID(), max(cgc, params.BeaconConfig().SamplesPerSlot))
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}
	return peerdas.NewColumnIndicesFromMap(peerInfo.CustodyColumns), nil
}

func buildColumnBatch(ctx context.Context, b batch, fuluBlocks verifiedROBlocks, p p2p.P2P) (*columnBatch, error) {
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

	indices, err := currentCustodiedColumns(ctx, p)
	if err != nil {
		return nil, errors.Wrap(err, "current custodied columns")
	}

	summary := &columnBatch{
		custodyGroups: indices,
		toDownload:    make(map[[32]byte]*toDownload, len(fuluBlocks)),
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
		if len(summary.toDownload) == 0 {
			summary.first = slot
		}
		summary.toDownload[b.Root()] = &toDownload{
			remaining:   indices.Copy(),
			commitments: cmts,
		}
		summary.last = slot
	}

	return summary, nil
}
