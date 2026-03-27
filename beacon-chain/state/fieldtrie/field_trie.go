package fieldtrie

import (
	"fmt"
	"maps"
	"reflect"
	"runtime"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	multi_value_slice "github.com/OffchainLabs/prysm/v7/container/multi-value-slice"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	pmath "github.com/OffchainLabs/prysm/v7/math"
	"github.com/pkg/errors"
)

var (
	ErrInvalidFieldTrie = errors.New("invalid field trie")
	ErrEmptyFieldTrie   = errors.New("empty field trie")
)

// sliceAccessor describes an interface for a multivalue slice
// object that returns information about the multivalue slice along with the
// particular state instance we are referencing.
type sliceAccessor interface {
	Len(obj multi_value_slice.Identifiable) int
	State() multi_value_slice.Identifiable
}

// OverlayPromotionThreshold is the maximum number of overlay entries
// before an overlay is promoted to an owned trie. Epoch boundaries
// typically dirty all ~1.1M validators at once, so this threshold
// catches that case before adding entries to override maps.
// Exported so that callers (e.g. recomputeFieldTrie) can detect large
// dirty sets and choose a from-scratch rebuild instead.
const OverlayPromotionThreshold = 10000

// FieldTrie is the representation of the representative
// trie of the particular field.
//
// A FieldTrie operates in one of two modes:
//   - Owned mode: nodes != nil, base == nil. The trie owns its full
//     layer data as a contiguous flat buffer and mutations happen in-place.
//   - Overlay mode: nodes == nil, base != nil. The trie stores only
//     sparse diffs (overrides) against an immutable base trie. Root computation
//     walks the base read-only, substituting override values at modified positions.
type FieldTrie struct {
	*sync.RWMutex
	reference     *stateutil.Reference
	nodes         [][32]byte            // all levels packed contiguously (owned mode)
	offsets       []int                 // offsets[i] = start of level i; offsets[depth+1] = len(nodes)
	base          *FieldTrie            // non-nil in overlay mode; immutable, ref-counted
	overrides     []map[uint64][32]byte // per-level sparse modifications [level][nodeIdx] → hash
	field         types.FieldIndex
	dataType      types.DataType
	length        uint64
	numOfElems    int
	isTransferred bool
	metrics       *metricsRef
}

// depth returns the trie depth from the offsets table.
func (f *FieldTrie) depth() int {
	return len(f.offsets) - 2
}

// levelSize returns the number of nodes at the given level.
func (f *FieldTrie) levelSize(level int) int {
	return f.offsets[level+1] - f.offsets[level]
}

// NewFieldTrie is the constructor for the field trie data structure. It creates the corresponding
// trie according to the given parameters. Depending on whether the field is a basic/composite array
// which is either fixed/variable length, it will appropriately determine the trie.
func NewFieldTrie(field types.FieldIndex, fieldInfo types.DataType, elements any, length uint64) (*FieldTrie, error) {
	if elements == nil {
		ft := &FieldTrie{
			field:      field,
			dataType:   fieldInfo,
			reference:  stateutil.NewRef(1),
			RWMutex:    new(sync.RWMutex),
			length:     length,
			numOfElems: 0,
			metrics:    &metricsRef{field: field},
		}

		ft.addCleanup()

		return ft, nil
	}

	fieldRoots, err := fieldConverters(field, []uint64{}, elements, true)
	if err != nil {
		return nil, err
	}

	if err := validateElements(field, fieldInfo, elements, length); err != nil {
		return nil, err
	}
	var numOfElems int
	if val, ok := elements.(sliceAccessor); ok {
		numOfElems = val.Len(val.State())
	} else {
		numOfElems = reflect.Indirect(reflect.ValueOf(elements)).Len()
	}
	switch fieldInfo {
	case types.BasicArray:
		nodes, offsets, err := stateutil.ReturnTrieLayer(fieldRoots, length)
		if err != nil {
			return nil, err
		}
		ft := &FieldTrie{
			nodes:      nodes,
			offsets:    offsets,
			field:      field,
			dataType:   fieldInfo,
			reference:  stateutil.NewRef(1),
			RWMutex:    new(sync.RWMutex),
			length:     length,
			numOfElems: numOfElems,
			metrics:    &metricsRef{field: field},
		}

		ft.updateMetrics(ft.nodeSize(), ft.overlaySize())
		ft.addCleanup()

		return ft, nil
	case types.CompositeArray, types.CompressedArray:
		nodes, offsets := stateutil.ReturnTrieLayerVariable(fieldRoots, length)
		ft := &FieldTrie{
			nodes:      nodes,
			offsets:    offsets,
			field:      field,
			dataType:   fieldInfo,
			reference:  stateutil.NewRef(1),
			RWMutex:    new(sync.RWMutex),
			length:     length,
			numOfElems: numOfElems,
			metrics:    &metricsRef{field: field},
		}

		ft.updateMetrics(ft.nodeSize(), ft.overlaySize())
		ft.addCleanup()

		return ft, nil
	default:
		return nil, errors.Errorf("unrecognized data type in field map: %v", reflect.TypeFor[types.DataType]().Name())
	}
}

// RecomputeTrie rebuilds the affected branches in the trie according to the provided
// changed indices and elements. This recomputes the trie according to the particular
// field the trie is based on.
func (f *FieldTrie) RecomputeTrie(indices []uint64, elements any) ([32]byte, error) {
	f.Lock()
	defer f.Unlock()
	if len(indices) == 0 {
		return f.TrieRoot()
	}

	fieldRoots, err := fieldConverters(f.field, indices, elements, false)
	if err != nil {
		return [32]byte{}, err
	}

	if err := f.validateIndices(indices); err != nil {
		return [32]byte{}, err
	}
	if val, ok := elements.(sliceAccessor); ok {
		f.numOfElems = val.Len(val.State())
	} else {
		f.numOfElems = reflect.Indirect(reflect.ValueOf(elements)).Len()
	}

	// Dispatch to overlay or owned recomputation.
	oldNodeSize, oldOverrideSize := f.nodeSize(), f.overlaySize()

	if f.base != nil {
		root, err := f.recomputeOverlayDispatch(indices, fieldRoots)
		if err != nil {
			return [32]byte{}, fmt.Errorf("recompute overlay dispatch: %w", err)
		}
		f.updateMetrics(f.nodeSize()-oldNodeSize, f.overlaySize()-oldOverrideSize)
		return root, nil
	}

	root, err := f.recomputeOwned(indices, fieldRoots)
	if err != nil {
		return [32]byte{}, fmt.Errorf("recompute owned: %w", err)
	}

	deltaNodeSize := f.nodeSize() - oldNodeSize
	deltaOverrideSize := f.overlaySize() - oldOverrideSize
	f.updateMetrics(deltaNodeSize, deltaOverrideSize)

	return root, nil
}

// CopyTrie creates a lightweight overlay copy of the trie. Instead of
// deep-copying all layer data, the copy shares the immutable base and
// stores only sparse diffs. This is O(1) for a fresh copy from an owned
// trie, or O(K) where K is the number of existing overrides when copying
// from another overlay.
func (f *FieldTrie) CopyTrie() *FieldTrie {
	var cp *FieldTrie
	if f.Empty() {
		cp = &FieldTrie{
			field:      f.field,
			dataType:   f.dataType,
			reference:  stateutil.NewRef(1),
			RWMutex:    new(sync.RWMutex),
			length:     f.length,
			numOfElems: f.numOfElems,
			metrics:    &metricsRef{field: f.field},
		}
	} else if f.base != nil {
		// Source is an overlay: new overlay sharing the same base.
		f.base.reference.AddRef()
		cp = &FieldTrie{
			base:       f.base,
			overrides:  copyOverrides(f.overrides),
			field:      f.field,
			dataType:   f.dataType,
			reference:  stateutil.NewRef(1),
			RWMutex:    new(sync.RWMutex),
			length:     f.length,
			numOfElems: f.numOfElems,
			metrics:    &metricsRef{field: f.field},
		}
	} else {
		// Source is owned: create an overlay on this trie.
		f.reference.AddRef()
		d := f.depth()
		overrides := make([]map[uint64][32]byte, d+1)
		cp = &FieldTrie{
			base:       f,
			overrides:  overrides,
			field:      f.field,
			dataType:   f.dataType,
			reference:  stateutil.NewRef(1),
			RWMutex:    new(sync.RWMutex),
			length:     f.length,
			numOfElems: f.numOfElems,
			metrics:    &metricsRef{field: f.field},
		}
	}

	cp.updateMetrics(cp.nodeSize(), cp.overlaySize())
	cp.addCleanup()

	return cp
}

// Length return the length of the whole field trie.
func (f *FieldTrie) Length() uint64 {
	return f.length
}

// TrieRoot returns the corresponding root of the trie.
func (f *FieldTrie) TrieRoot() ([32]byte, error) {
	if f.Empty() {
		return [32]byte{}, ErrEmptyFieldTrie
	}

	// Overlay mode: read root from overrides, fallback to base.
	if f.base != nil {
		depth := f.base.depth()
		trieRoot, err := f.readOverlayNode(depth, 0)
		if err != nil {
			return [32]byte{}, err
		}
		switch f.dataType {
		case types.BasicArray:
			return trieRoot, nil
		case types.CompositeArray:
			leafCount := uint64(f.base.numOfElems)
			for idx := range f.overrides[0] {
				if idx+1 > leafCount {
					leafCount = idx + 1
				}
			}
			return stateutil.AddInMixin(trieRoot, leafCount)
		case types.CompressedArray:
			return stateutil.AddInMixin(trieRoot, uint64(f.numOfElems))
		default:
			return [32]byte{}, errors.Errorf("unrecognized data type in field map: %v", reflect.TypeFor[types.DataType]().Name())
		}
	}

	// Owned mode.
	depth := f.depth()
	if f.levelSize(depth) == 0 {
		return [32]byte{}, ErrInvalidFieldTrie
	}
	trieRoot := f.nodes[f.offsets[depth]]
	switch f.dataType {
	case types.BasicArray:
		return trieRoot, nil
	case types.CompositeArray:
		return stateutil.AddInMixin(trieRoot, uint64(f.numOfElems))
	case types.CompressedArray:
		return stateutil.AddInMixin(trieRoot, uint64(f.numOfElems))
	default:
		return [32]byte{}, errors.Errorf("unrecognized data type in field map: %v", reflect.TypeFor[types.DataType]().Name())
	}
}

// FieldReference returns the underlying field reference
// object for the trie.
func (f *FieldTrie) FieldReference() *stateutil.Reference {
	return f.reference
}

// Empty checks whether the underlying field trie is
// empty or not.
func (f *FieldTrie) Empty() bool {
	return f == nil || (f.nodes == nil && f.base == nil) || f.isTransferred
}

// IsOverlay returns true if this trie operates in overlay mode,
// storing sparse diffs against an immutable base.
func (f *FieldTrie) IsOverlay() bool {
	return f != nil && f.base != nil
}

// ReleaseBase decrements the base trie's reference count and clears
// the overlay's reference to it. Called during finalizer cleanup to
// allow the base to be garbage collected when no overlays reference it.
func (f *FieldTrie) ReleaseBase() {
	if f.base != nil {
		oldOverrides := f.overlaySize()
		f.base.reference.MinusRef()
		f.base = nil
		f.overrides = nil
		f.updateMetrics(0, -oldOverrides)
	}
}

// nodeSize returns the total number of node entries in the trie.
func (f *FieldTrie) nodeSize() int {
	return len(f.nodes)
}

// overlaySize returns the total number of entries across all override maps.
func (f *FieldTrie) overlaySize() int {
	n := 0
	for _, m := range f.overrides {
		n += len(m)
	}
	return n
}

// copyOverrides returns a deep copy of the override maps.
func copyOverrides(src []map[uint64][32]byte) []map[uint64][32]byte {
	if src == nil {
		return nil
	}
	dst := make([]map[uint64][32]byte, len(src))
	for i, m := range src {
		if len(m) > 0 {
			dst[i] = make(map[uint64][32]byte, len(m))
			maps.Copy(dst[i], m)
		}
	}
	return dst
}

// readOverlayNode reads a node from the overlay at (level, idx).
// Priority: overrides → base.nodes → trie.ZeroHashes.
func (f *FieldTrie) readOverlayNode(level int, idx uint64) ([32]byte, error) {
	if m := f.overrides[level]; m != nil {
		if v, ok := m[idx]; ok {
			return v, nil
		}
	}
	ii, err := pmath.Int(idx)
	if err != nil {
		return [32]byte{}, err
	}
	levelSize := f.base.levelSize(level)
	if ii < levelSize {
		return f.base.nodes[f.base.offsets[level]+ii], nil
	}
	return trie.ZeroHashes[level], nil
}

// recomputeOverlay walks up the trie from dirty leaves, hashing pairs
// and storing results in overrides. Returns the new root hash.
// dirtyLeaves maps leaf index → leaf hash at level 0.
func (f *FieldTrie) recomputeOverlay(dirtyLeaves map[uint64][32]byte) ([32]byte, error) {
	depth := len(f.overrides)
	hasher := hash.CustomSHA256Hasher()
	var combinedChunks [64]byte

	// Store dirty leaves in overrides[0].
	if f.overrides[0] == nil {
		f.overrides[0] = make(map[uint64][32]byte, len(dirtyLeaves))
	}
	maps.Copy(f.overrides[0], dirtyLeaves)

	// Walk up from level 0 to depth-1.
	currentDirty := dirtyLeaves
	for level := range depth - 1 {
		parentDirty := make(map[uint64][32]byte, len(currentDirty)/2+1)
		for idx := range currentDirty {
			parentIdx := idx / 2
			if _, done := parentDirty[parentIdx]; done {
				continue
			}
			leftIdx := parentIdx * 2
			rightIdx := leftIdx + 1

			left, err := f.readOverlayNode(level, leftIdx)
			if err != nil {
				return [32]byte{}, err
			}
			right, err := f.readOverlayNode(level, rightIdx)
			if err != nil {
				return [32]byte{}, err
			}

			copy(combinedChunks[:32], left[:])
			copy(combinedChunks[32:], right[:])
			parentHash := hasher(combinedChunks[:])

			parentDirty[parentIdx] = parentHash
			if f.overrides[level+1] == nil {
				f.overrides[level+1] = make(map[uint64][32]byte)
			}
			f.overrides[level+1][parentIdx] = parentHash
		}
		currentDirty = parentDirty
	}

	// The root is at overrides[depth-1][0], or fallback to base.
	return f.readOverlayNode(depth-1, 0)
}

// recomputeOverlayDispatch handles DataType-specific preprocessing before
// calling recomputeOverlay for the core walk-up.
func (f *FieldTrie) recomputeOverlayDispatch(indices []uint64, fieldRoots [][32]byte) ([32]byte, error) {
	// Check promotion threshold before adding to overrides.
	// Use leaf-level overlay count (overrides[0]) rather than overlaySize()
	// which sums across all trie levels. Each dirty leaf propagates entries
	// to ~depth levels during recomputeOverlay, so overlaySize() grows at
	// ~2× the leaf rate. For deep tries (validators: depth=40), this causes
	// premature rebuilds when only a small fraction of leaves changed.
	leafOverrides := len(f.overrides[0])
	if len(indices) > OverlayPromotionThreshold || leafOverrides > OverlayPromotionThreshold {
		return f.rebuildTrie(indices, fieldRoots)
	}

	switch f.dataType {
	case types.BasicArray:
		dirtyLeaves := make(map[uint64][32]byte, len(indices))
		for i, idx := range indices {
			dirtyLeaves[idx] = fieldRoots[i]
		}
		root, err := f.recomputeOverlay(dirtyLeaves)
		if err != nil {
			return [32]byte{}, err
		}
		return root, nil

	case types.CompositeArray:
		dirtyLeaves := make(map[uint64][32]byte, len(indices))
		for i, idx := range indices {
			dirtyLeaves[idx] = fieldRoots[i]
		}
		root, err := f.recomputeOverlay(dirtyLeaves)
		if err != nil {
			return [32]byte{}, err
		}
		// Leaf count: max of base numOfElems and any override indices.
		leafCount := uint64(f.base.numOfElems)
		for idx := range f.overrides[0] {
			if idx+1 > leafCount {
				leafCount = idx + 1
			}
		}
		return stateutil.AddInMixin(root, leafCount)

	case types.CompressedArray:
		numOfElems, err := f.field.ElemsInChunk()
		if err != nil {
			return [32]byte{}, err
		}
		// Deduplicate chunk indices.
		dirtyLeaves := make(map[uint64][32]byte, len(indices))
		for i, idx := range indices {
			chunkIdx := idx / numOfElems
			if _, exists := dirtyLeaves[chunkIdx]; !exists {
				dirtyLeaves[chunkIdx] = fieldRoots[i]
			}
		}
		root, err := f.recomputeOverlay(dirtyLeaves)
		if err != nil {
			return [32]byte{}, err
		}
		return stateutil.AddInMixin(root, uint64(f.numOfElems))

	default:
		return [32]byte{}, errors.Errorf("unrecognized data type in field map: %v", reflect.TypeFor[types.DataType]().Name())
	}
}

// rebuildTrie replaces promoteToOwned + recomputeOwned for the overlay promotion path.
// Instead of copying the entire base buffer (which recomputeOwned then overwrites),
// it allocates a fresh buffer, fills level 0, and vectorized-hashes all upper levels.
// For the all-dirty case (epoch boundaries), this eliminates the base copy entirely.
func (f *FieldTrie) rebuildTrie(indices []uint64, fieldRoots [][32]byte) ([32]byte, error) {
	depth := f.base.depth()

	// Determine leaf-level indices and roots based on data type.
	leafIndices := indices
	leafRoots := fieldRoots
	if f.dataType == types.CompressedArray {
		numOfElems, err := f.field.ElemsInChunk()
		if err != nil {
			return [32]byte{}, err
		}
		// Deduplicate to chunk-level indices.
		seen := make(map[uint64]bool, len(indices))
		chunkIndices := make([]uint64, 0, len(indices))
		chunkRoots := make([][32]byte, 0, len(indices))
		for i, idx := range indices {
			chunkIdx := idx / numOfElems
			if seen[chunkIdx] {
				continue
			}
			seen[chunkIdx] = true
			chunkIndices = append(chunkIndices, chunkIdx)
			chunkRoots = append(chunkRoots, fieldRoots[i])
		}
		leafIndices = chunkIndices
		leafRoots = chunkRoots
	}

	// Determine the leaf count for the new buffer.
	leafCount := f.numOfElems
	if f.dataType == types.CompressedArray {
		leafCount = f.base.levelSize(0)
	}
	for _, idx := range leafIndices {
		ii, err := pmath.Int(idx)
		if err != nil {
			return [32]byte{}, err
		}
		if ii+1 > leafCount {
			leafCount = ii + 1
		}
	}

	// Allocate fresh buffer.
	f.offsets = stateutil.ComputeOffsetsVariable(depth, leafCount)
	f.nodes = make([][32]byte, f.offsets[depth+1])

	// Fill level 0. Skip the base copy when all leaves are being rewritten.
	allDirty := len(leafIndices) >= leafCount
	if !allDirty {
		// Partial: seed from base level 0 + accumulated overrides.
		baseL0 := f.base.levelSize(0)
		copy(f.nodes[:baseL0], f.base.nodes[f.base.offsets[0]:f.base.offsets[0]+baseL0])
		for idx, val := range f.overrides[0] {
			ii, err := pmath.Int(idx)
			if err != nil {
				return [32]byte{}, err
			}
			f.nodes[ii] = val
		}
	}
	// Scatter current changes into level 0.
	for i, idx := range leafIndices {
		ii, err := pmath.Int(idx)
		if err != nil {
			return [32]byte{}, err
		}
		f.nodes[ii] = leafRoots[i]
	}

	// Vectorized-hash all upper levels from level 0.
	stateutil.HashUpFromLeaves(f.nodes, f.offsets)

	// Release the base.
	f.base.reference.MinusRef()
	f.base = nil
	f.overrides = nil

	// Return root with appropriate mixin.
	trieRoot := f.nodes[f.offsets[depth]]
	switch f.dataType {
	case types.BasicArray:
		return trieRoot, nil
	case types.CompositeArray:
		return stateutil.AddInMixin(trieRoot, uint64(f.numOfElems))
	case types.CompressedArray:
		return stateutil.AddInMixin(trieRoot, uint64(f.numOfElems))
	default:
		return [32]byte{}, errors.Errorf("unrecognized data type in field map: %v", reflect.TypeFor[types.DataType]().Name())
	}
}

// recomputeOwned handles trie recomputation for an owned-mode trie.
// This is the original RecomputeTrie logic, extracted for use after promotion.
func (f *FieldTrie) recomputeOwned(indices []uint64, fieldRoots [][32]byte) ([32]byte, error) {
	var fieldRoot [32]byte
	var err error
	switch f.dataType {
	case types.BasicArray:
		fieldRoot, err = stateutil.RecomputeFromLayer(fieldRoots, indices, f.nodes, f.offsets)
		if err != nil {
			return [32]byte{}, err
		}
		return fieldRoot, nil
	case types.CompositeArray:
		fieldRoot, f.nodes, f.offsets, err = stateutil.RecomputeFromLayerVariable(fieldRoots, indices, f.nodes, f.offsets)
		if err != nil {
			return [32]byte{}, err
		}
		return stateutil.AddInMixin(fieldRoot, uint64(f.numOfElems))
	case types.CompressedArray:
		numOfElems, err := f.field.ElemsInChunk()
		if err != nil {
			return [32]byte{}, err
		}
		iNumOfElems, err := pmath.Int(numOfElems)
		if err != nil {
			return [32]byte{}, err
		}
		var newIndices []uint64
		indexExists := make(map[uint64]bool)
		newRoots := make([][32]byte, 0, len(fieldRoots)/iNumOfElems)
		for i, idx := range indices {
			startIdx := idx / numOfElems
			if indexExists[startIdx] {
				continue
			}
			newIndices = append(newIndices, startIdx)
			indexExists[startIdx] = true
			newRoots = append(newRoots, fieldRoots[i])
		}
		fieldRoot, f.nodes, f.offsets, err = stateutil.RecomputeFromLayerVariable(newRoots, newIndices, f.nodes, f.offsets)
		if err != nil {
			return [32]byte{}, err
		}
		return stateutil.AddInMixin(fieldRoot, uint64(f.numOfElems))
	default:
		return [32]byte{}, errors.Errorf("unrecognized data type in field map: %v", reflect.TypeFor[types.DataType]().Name())
	}
}

// promoteToOwned converts an overlay trie to an owned trie by copying
// the base's flat buffer and applying all overrides on top.
func (f *FieldTrie) promoteToOwned() error {
	// Determine if overrides require a larger buffer than the base.
	baseCap := f.base.levelSize(0)
	maxLeafIdx := baseCap - 1
	if f.overrides[0] != nil {
		for idx := range f.overrides[0] {
			ii, err := pmath.Int(idx)
			if err != nil {
				return err
			}
			if ii > maxLeafIdx {
				maxLeafIdx = ii
			}
		}
	}

	if maxLeafIdx+1 > baseCap {
		// Growth needed: allocate larger buffer with base data copied.
		f.nodes, f.offsets = stateutil.GrowFlatBuffer(f.base.nodes, f.base.offsets, maxLeafIdx+1)
	} else {
		// Copy base buffer directly.
		f.nodes = make([][32]byte, len(f.base.nodes))
		copy(f.nodes, f.base.nodes)
		f.offsets = make([]int, len(f.base.offsets))
		copy(f.offsets, f.base.offsets)
	}

	// Apply all overrides: direct value writes, zero allocations.
	for level, m := range f.overrides {
		for idx, val := range m {
			ii, err := pmath.Int(idx)
			if err != nil {
				return err
			}
			f.nodes[f.offsets[level]+ii] = val
		}
	}

	f.base.reference.MinusRef()
	f.base = nil
	f.overrides = nil
	return nil
}

// updateMetrics adds the given deltas (in number of [32]byte entries) to the gauges
// and keeps the cleanup reference in sync.
func (f *FieldTrie) updateMetrics(nodesDelta, overridesDelta int) {
	label := f.field.String()
	fieldTrieNodesBytesGauge.WithLabelValues(label).Add(float64(nodesDelta * 32))
	fieldTrieOverridesBytesGauge.WithLabelValues(label).Add(float64(overridesDelta * 32))
	f.metrics.nodes += nodesDelta
	f.metrics.overrides += overridesDelta
}

// metricsRef holds the current metric contribution for a FieldTrie,
// stored separately so runtime.AddCleanup can access it without
// preventing the FieldTrie from being collected.
type metricsRef struct {
	field     types.FieldIndex
	nodes     int
	overrides int
}

// addCleanup registers a cleanup function that subtracts this trie's
// contribution from the gauges when it is garbage collected.
func (f *FieldTrie) addCleanup() {
	runtime.AddCleanup(f, func(m *metricsRef) {
		label := m.field.String()
		fieldTrieNodesBytesGauge.WithLabelValues(label).Sub(float64(m.nodes * 32))
		fieldTrieOverridesBytesGauge.WithLabelValues(label).Sub(float64(m.overrides * 32))
	}, f.metrics)
}

// InsertFlatLayers manually inserts flat trie data. This method
// bypasses the normal method of field computation, it is only
// meant to be used in tests.
func (f *FieldTrie) InsertFlatLayers(nodes [][32]byte, offsets []int) {
	f.nodes = nodes
	f.offsets = offsets
}
