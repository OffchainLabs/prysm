package backfill

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

var (
	errInvalidDataColumnResponse = errors.New("invalid DataColumnSidecar response")
	errUnexpectedBlockRoot       = errors.Wrap(errInvalidDataColumnResponse, "unexpected sidecar block root")
	errCommitmentLengthMismatch  = errors.Wrap(errInvalidDataColumnResponse, "sidecar has different commitment count than block")
	errCommitmentValueMismatch   = errors.Wrap(errInvalidDataColumnResponse, "sidecar commitments do not match block")
	errSidecarSignatureMismatch  = errors.Wrap(errInvalidDataColumnResponse, "sidecar signed block header signature does not match block")
)

// tune the amount of columns we try to download from peers at once.
// The spec limit is 128 * 32, but connection errors are more likely when
// requesting so much at once.
const columnRequestLimit = 128 * 4

// payloadFullness classifies whether a block's execution payload was revealed on chain.
// Pre-gloas payloads are always revealed. A gloas payload may be withheld, in which case no
// data columns exist for the block; the canonical child's bid testifies either way.
type payloadFullness int

const (
	// fullnessUnknown means no canonical child was available to testify; only the batch tail can stay unknown.
	fullnessUnknown payloadFullness = iota
	// fullnessRevealed means the payload was revealed, so custody columns are required.
	fullnessRevealed
	// fullnessWithheld means the payload was withheld, so there are no columns to require.
	fullnessWithheld
)

// boundaryChildFn looks up the already-imported canonical child of the given root, returning nil when it isn't known.
type boundaryChildFn func(ctx context.Context, parentRoot [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error)

type columnBatch struct {
	first         primitives.Slot
	last          primitives.Slot
	custodyGroups peerdas.ColumnIndices
	toDownload    map[[32]byte]*toDownload
}

type toDownload struct {
	remaining      peerdas.ColumnIndices
	commitments    [][]byte
	slot           primitives.Slot
	blockSignature [fieldparams.BLSSignatureLength]byte
	fullness       payloadFullness
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

// pruneExpired removes any columns from the batch that are no longer needed.
// If `pruned` is non-nil, it is populated with the roots that were removed.
func (cs *columnBatch) pruneExpired(needs das.CurrentNeeds, pruned map[[32]byte]struct{}) {
	var first, last primitives.Slot
	hasTrackedBlock := false
	for root, td := range cs.toDownload {
		if !needs.Col.At(td.slot) {
			delete(cs.toDownload, root)
			if pruned != nil {
				pruned[root] = struct{}{}
			}
			continue
		}
		if !hasTrackedBlock || td.slot < first {
			first = td.slot
		}
		if !hasTrackedBlock || td.slot > last {
			last = td.slot
		}
		hasTrackedBlock = true
	}
	// Keep the request range aligned with toDownload. In particular, do not request roots
	// that were just deleted: a peer may still serve them and the validator would reject them
	// as unexpected. The zero values are harmless when every tracked block was pruned because
	// no column request will be made.
	cs.first = first
	cs.last = last
}

// neededSidecarCount returns the total number of sidecars still needed to complete the batch.
func (cs *columnBatch) neededSidecarCount() int {
	count := 0
	for _, v := range cs.toDownload {
		count += v.remaining.Count()
	}
	return count
}

// neededSidecarsByColumn counts how many sidecars are still needed for each column index.
func (cs *columnBatch) neededSidecarsByColumn(peerHas peerdas.ColumnIndices) map[uint64]int {
	need := make(map[uint64]int, len(peerHas))
	for _, v := range cs.toDownload {
		for idx := range v.remaining {
			if peerHas.Has(idx) {
				need[idx]++
			}
		}
	}
	return need
}

type columnSync struct {
	*columnBatch
	store    *das.LazilyPersistentStoreColumn
	current  primitives.Slot
	peer     peer.ID
	bisector *columnBisector
}

func newColumnSync(ctx context.Context, b batch, blks verifiedROBlocks, current primitives.Slot, p p2p.P2P, cfg *workerCfg) (*columnSync, error) {
	cgc, err := p.CustodyGroupCount(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "custody group count")
	}
	cb, err := buildColumnBatch(ctx, b, blks, p, cfg.colStore, cfg.currentNeeds(), cfg.boundaryChild)
	if err != nil {
		return nil, err
	}
	if cb == nil {
		return &columnSync{}, nil
	}
	shouldRetain := func(sl primitives.Slot) bool {
		needs := cfg.currentNeeds()
		return needs.Col.At(sl)
	}

	bisector := newColumnBisector(cfg.downscore)
	return &columnSync{
		columnBatch: cb,
		current:     current,
		store:       das.NewLazilyPersistentStoreColumn(cfg.colStore, cfg.newVC, p.NodeID(), cgc, bisector, shouldRetain),
		bisector:    bisector,
	}, nil
}

func (cs *columnSync) blockColumns(root [32]byte) *toDownload {
	if cs == nil || cs.columnBatch == nil {
		return nil
	}
	return cs.columnBatch.toDownload[root]
}

// fullness returns the payload fullness recorded for the given block root, or unknown if untracked.
func (cs *columnSync) fullness(root [32]byte) payloadFullness {
	td := cs.blockColumns(root)
	if td == nil {
		return fullnessUnknown
	}
	return td.fullness
}

func (cs *columnSync) columnsNeeded() peerdas.ColumnIndices {
	if cs == nil || cs.columnBatch == nil {
		return peerdas.ColumnIndices{}
	}
	return cs.columnBatch.needed()
}

func (cs *columnSync) request(reqCols []uint64, limit int) (*ethpb.DataColumnSidecarsByRangeRequest, error) {
	if len(reqCols) == 0 {
		return nil, nil
	}

	// Use cheaper check to avoid allocating map and counting sidecars if under limit.
	if cs.neededSidecarCount() <= limit {
		return sync.DataColumnSidecarsByRangeRequest(reqCols, cs.first, cs.last)
	}

	// Re-slice b.nextReqCols to keep the number of requested sidecars under the limit.
	reqCount := 0
	peerHas := peerdas.NewColumnIndicesFromSlice(reqCols)
	needed := cs.neededSidecarsByColumn(peerHas)
	for i := range reqCols {
		addSidecars := needed[reqCols[i]]
		if reqCount+addSidecars > columnRequestLimit {
			reqCols = reqCols[:i]
			break
		}
		reqCount += addSidecars
	}
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
		dataColumnSidecarDownloadCount.WithLabelValues(fmt.Sprintf("%d", cd.Index()), validity).Inc()
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
	if !expected.remaining.Has(cd.Index()) {
		return nil
	}
	// Gloas sidecars carry no commitments on the wire; seed them from the block's bid before reading them.
	if cd.IsGloas() {
		cd.SetBidCommitments(expected.commitments)
	}
	comms, err := cd.KzgCommitments()
	if err != nil {
		return err
	}
	if len(comms) != len(expected.commitments) {
		return errors.Wrapf(errCommitmentLengthMismatch, "root=%#x, slot=%d, index=%d", root, cd.Slot(), cd.Index())
	}
	for i, cmt := range comms {
		if !bytes.Equal(cmt, expected.commitments[i]) {
			return errors.Wrapf(errCommitmentValueMismatch, "root=%#x, slot=%d, index=%d", root, cd.Slot(), cd.Index())
		}
	}

	// Cross-check the sidecar's embedded SignedBlockHeader signature against the
	// locally held block. Gloas sidecars carry no header on the wire, so skip them.
	if !cd.IsGloas() {
		sbh, err := cd.SignedBlockHeader()
		if err != nil {
			return fmt.Errorf("sidecar signed block header root=%#x, index=%d: %w", root, cd.Index(), err)
		}

		if !bytes.Equal(sbh.Signature, expected.blockSignature[:]) {
			return fmt.Errorf("root=%#x, slot=%d, index=%d: %w", root, cd.Slot(), cd.Index(), errSidecarSignatureMismatch)
		}
	}

	if err := v.columnSync.store.Persist(v.columnSync.current, cd); err != nil {
		return errors.Wrap(err, "persisting data column")
	}
	v.bisector.addPeerColumns(v.columnSync.peer, cd)
	expected.remaining.Unset(cd.Index())
	return nil
}

func currentCustodiedColumns(ctx context.Context, p p2p.P2P) (peerdas.ColumnIndices, error) {
	cgc, err := p.CustodyGroupCount(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "custody group count")
	}

	peerInfo, _, err := peerdas.Info(p.NodeID(), cgc)
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}
	return peerdas.NewColumnIndicesFromMap(peerInfo.CustodyColumns), nil
}

func buildColumnBatch(ctx context.Context, b batch, blks verifiedROBlocks, p p2p.P2P, store *filesystem.DataColumnStorage, needs das.CurrentNeeds, child boundaryChildFn) (*columnBatch, error) {
	if len(blks) == 0 {
		return nil, nil
	}

	if !needs.Col.At(b.begin) && !needs.Col.At(b.end-1) {
		return nil, nil
	}

	indices, err := currentCustodiedColumns(ctx, p)
	if err != nil {
		return nil, errors.Wrap(err, "current custodied columns")
	}
	summary := &columnBatch{
		custodyGroups: indices,
		toDownload:    make(map[[32]byte]*toDownload, len(blks)),
	}
	for i, b := range blks {
		slot := b.Block().Slot()
		if !needs.Col.At(slot) {
			continue
		}
		cmts, err := b.Block().Body().BlobKzgCommitments()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get blob kzg commitments")
		}
		if len(cmts) == 0 {
			continue
		}
		fullness, err := blockFullness(ctx, blks, i, child)
		if err != nil {
			return nil, err
		}
		var remaining peerdas.ColumnIndices
		if fullness == fullnessRevealed {
			remaining = das.IndicesNotStored(store.Summary(b.Root()), indices)
		} else {
			// Withheld and unresolved payloads have no columns to require, but keep the root known
			// so peers still serving columns for them are not penalized.
			remaining = peerdas.ColumnIndices{}
		}
		// The last block this part of the loop sees will be the last one
		// we need to download data columns for.
		if len(summary.toDownload) == 0 {
			// toDownload is only empty the first time through, so this is the first block with data columns.
			summary.first = slot
		}
		summary.last = slot
		summary.toDownload[b.Root()] = &toDownload{
			remaining:      remaining,
			commitments:    cmts,
			slot:           slot,
			blockSignature: b.Signature(),
			fullness:       fullness,
		}
	}

	return summary, nil
}

// blockFullness classifies the payload fullness of blks[i]. Batch blocks are parent-linked by
// verify, so blks[i+1] is the direct child even across skipped slots. The batch tail has no
// in-batch child; the boundary child (the already-imported block just above the batch) may
// testify, otherwise the tail stays unknown until import time rather than defaulting to revealed.
func blockFullness(ctx context.Context, blks verifiedROBlocks, i int, child boundaryChildFn) (payloadFullness, error) {
	blk := blks[i]
	if blk.Block().Version() < version.Gloas {
		return fullnessRevealed, nil
	}
	if i+1 < len(blks) {
		return fullnessFromChild(blk.Block(), blks[i+1].Block())
	}
	if child == nil {
		return fullnessUnknown, nil
	}
	cb, err := child(ctx, blk.Root())
	if err != nil {
		// The build-time lookup is best-effort; the importer authoritatively resolves the tail.
		log.WithError(err).WithField("root", fmt.Sprintf("%#x", blk.Root())).Debug("Boundary child lookup failed, leaving tail fullness unknown")
		return fullnessUnknown, nil
	}
	if cb == nil {
		return fullnessUnknown, nil
	}
	return fullnessFromChild(blk.Block(), cb.Block())
}

// fullnessFromChild classifies a gloas parent's payload fullness from its direct child's bid.
func fullnessFromChild(parent, child interfaces.ReadOnlyBeaconBlock) (payloadFullness, error) {
	full, err := blocks.BlockBuiltOnParentPayload(parent, child)
	if err != nil {
		return fullnessUnknown, errors.Wrap(err, "block built on parent payload")
	}
	if full {
		return fullnessRevealed, nil
	}
	return fullnessWithheld, nil
}
