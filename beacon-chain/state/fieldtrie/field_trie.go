package fieldtrie

import (
	"encoding/binary"
	"fmt"
	"maps"
	"reflect"
	"runtime"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	multi_value_slice "github.com/OffchainLabs/prysm/v7/container/multi-value-slice"
	"github.com/OffchainLabs/prysm/v7/container/slice"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var (
	ErrInvalidFieldTrie = errors.New("invalid field trie")
	ErrEmptyFieldTrie   = errors.New("empty field trie")
)

// overlayPromotionThreshold is the maximum number of dirty leaves
// before an overlay is promoted to a full trie rebuild.
//
// The overlay path costs O(k × depth) with per-entry map overhead,
// where k is the number of dirty leaves.
// The rebuild path costs O(n) with vectorized hashing, where n is
// the total number of leaves.
// At ~10K dirty leaves and depth ~40, the overlay's map-heavy random
// access starts to exceed the cost of a flat sequential rebuild over
// ~1M leaves. This threshold catches bulk mutations (e.g. epoch
// boundaries dirtying all ~1.1M validators) early, before populating
// expensive override maps.
const overlayPromotionThreshold = 10_000

type (
	// FieldTrie is the representation of the representative
	// trie of the particular field.
	//
	// A FieldTrie operates in one of two modes:
	//   - Owned mode: nodes != nil, base == nil. The trie owns its full
	//     layer data as a contiguous flat buffer and mutations happen in-place.
	//   - Overlay mode: nodes == nil, base != nil. The trie stores only
	//     sparse diffs (overrides) against an immutable base trie. Root computation
	//     walks the base read-only, substituting override values at modified positions.
	FieldTrie struct {
		mu      sync.RWMutex
		ref     *stateutil.Reference // count of holders (BeaconState copies) pointing to this FieldTrie
		dataRef *stateutil.Reference // count of overlay bases sharing this trie's nodes buffer
		cleanup runtime.Cleanup
		metrics *metricsRef

		// Owned mode fields:
		nodes   [][32]byte // flat buffer with all trie levels packed contiguously
		offsets []uint64   // maps each trie level to its start index in nodes. Also offsets[depth+1] = len(nodes)

		// Overlay mode fields:
		base      *FieldTrie            // immutable base trie (nil in owned mode), kept alive by Go's GC
		overrides []map[uint64][32]byte // per-level sparse diffs: overrides[level][nodeIdx] = hash

		// Field metadata:
		field      types.FieldIndex // which beacon state field this trie represents
		dataType   types.DataType   // encoding: BasicArray, CompositeArray, or CompressedArray
		length     uint64           // maximum capacity (e.g., VALIDATOR_REGISTRY_LIMIT); determines trie depth
		numOfElems uint64           // current number of elems in the field
	}

	// sliceAccessor describes an interface for a multivalue slice
	// object that returns information about the multivalue slice along with the
	// particular state instance we are referencing.
	sliceAccessor interface {
		Len(obj multi_value_slice.Identifiable) int
		State() multi_value_slice.Identifiable
	}

	// metricsRef holds the current metric contribution for a FieldTrie,
	// stored separately so runtime.AddCleanup can access it without
	// preventing the FieldTrie from being collected.
	metricsRef struct {
		field         types.FieldIndex
		nodes         int
		overrides     int
		leafOverrides int
		overlay       bool
	}
)

// NewFieldTrie creates a new field trie from the given elements.
// length is the maximum capacity of the field (e.g., VALIDATOR_REGISTRY_LIMIT) and determines
// the trie depth. The number of elements must be <= length.
func NewFieldTrie(field types.FieldIndex, fieldInfo types.DataType, elements any, length uint64) (*FieldTrie, error) {
	if !map[types.DataType]bool{
		types.BasicArray:      true,
		types.CompositeArray:  true,
		types.CompressedArray: true,
	}[fieldInfo] {
		return nil, errors.Errorf("unrecognized data type in field map: %v", reflect.TypeFor[types.DataType]().Name())
	}

	if err := validateElements(field, fieldInfo, elements, length); err != nil {
		return nil, fmt.Errorf("validate elements: %w", err)
	}

	nodes, offsets, err := buildTrie(field, elements, length)
	if err != nil {
		return nil, fmt.Errorf("build trie: %w", err)
	}

	fieldTrie := &FieldTrie{
		ref:        stateutil.NewRef(1),
		dataRef:    stateutil.NewRef(0),
		nodes:      nodes,
		offsets:    offsets,
		field:      field,
		dataType:   fieldInfo,
		length:     length,
		numOfElems: elemCount(elements),
	}

	if !fieldTrie.empty() {
		fieldTrie.updateMetrics()
		fieldTrie.cleanup = runtime.AddCleanup(fieldTrie, cleanupMetrics, fieldTrie.metrics)
	}

	logBalancesTrieCreation(field, "NewFieldTrie")

	return fieldTrie, nil
}

// TrieRoot returns the root of the trie with the appropriate length mixin applied.
func (f *FieldTrie) TrieRoot() ([32]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.trieRoot()
}

// CopyTrie creates a copy of the trie.
func (f *FieldTrie) CopyTrie() *FieldTrie {
	f.mu.RLock()
	defer f.mu.RUnlock()

	mode := overlayMode(f.base != nil)
	fieldTrieCopyCounter.WithLabelValues(f.field.String(), mode).Inc()

	f.ref.AddRef()
	return f
}

// RecomputeTrie recomputes the trie for the given changed indices and returns
// the new trie and root hash. The caller MUST use the returned *FieldTrie
// in place of the one on which this method was called, even if an error
// is returned.
func (f *FieldTrie) RecomputeTrie(indices []uint64, elements any) (*FieldTrie, [32]byte, error) {
	indices = slice.SetUint64(indices)

	f.mu.Lock()
	defer f.mu.Unlock()

	// If no changes, return existing root (read-only).
	if !f.empty() && len(indices) == 0 {
		root, err := f.trieRoot()
		return f, root, err
	}

	// Fork if shared: snapshot source data under the lock, then recompute on the fork.
	if f.isShared() {
		f.ref.MinusRef()
		fieldTrieForkCounter.WithLabelValues(f.field.String()).Inc()
		forked := f.fork()

		root, err := forked.recomputeInPlace(indices, elements)
		if err != nil {
			return forked, [32]byte{}, fmt.Errorf("recompute in place after fork: %w", err)
		}

		return forked, root, nil
	}

	root, err := f.recomputeInPlace(indices, elements)
	if err != nil {
		return f, [32]byte{}, fmt.Errorf("recompute in place: %w", err)
	}

	return f, root, nil
}

func (f *FieldTrie) trieRoot() ([32]byte, error) {
	if f.empty() {
		return [32]byte{}, ErrEmptyFieldTrie
	}

	// Owned mode: Directly read root from nodes.
	if f.base == nil {
		depth := f.depth()
		if f.levelSize(depth) == 0 {
			return [32]byte{}, ErrInvalidFieldTrie
		}

		rootOffset := f.offsets[depth]
		root := f.nodes[rootOffset]
		rootWithMixin, err := f.rootWithMixin(root)
		if err != nil {
			return [32]byte{}, fmt.Errorf("root with mixin: %w", err)
		}

		return rootWithMixin, nil
	}

	// Overlay mode: Read root from overrides and fallback to base.
	root, err := f.readOverlayNode(f.base.depth(), 0)
	if err != nil {
		return [32]byte{}, fmt.Errorf("read overlay node: %w", err)
	}

	rootWithMixin, err := f.rootWithMixin(root)
	if err != nil {
		return [32]byte{}, fmt.Errorf("root with mixin: %w", err)
	}

	return rootWithMixin, nil
}

// fork creates a new independent trie from the shared source's data.
func (f *FieldTrie) fork() *FieldTrie {
	forked := &FieldTrie{
		ref:        stateutil.NewRef(1),
		dataRef:    stateutil.NewRef(0),
		field:      f.field,
		dataType:   f.dataType,
		length:     f.length,
		numOfElems: f.numOfElems,
	}

	if f.empty() {
		return forked
	}

	if f.base == nil {
		// Owned mode: wrap source's nodes as immutable base for the fork.
		// The base shares the source's nodes slice, so we increment the
		// source's ref to prevent in-place mutation while the base exists.
		// A GC cleanup on the base decrements the source's ref when the
		// base (and all its overlays) are collected.
		depth := f.depth()
		base := &FieldTrie{
			ref:        stateutil.NewRef(1),
			dataRef:    stateutil.NewRef(0),
			nodes:      f.nodes,
			offsets:    f.offsets,
			field:      f.field,
			dataType:   f.dataType,
			length:     f.length,
			numOfElems: f.numOfElems,
		}

		f.dataRef.AddRef()
		base.updateMetrics()
		base.cleanup = runtime.AddCleanup(base, cleanupMetrics, base.metrics)
		runtime.AddCleanup(base, cleanupRef, f.dataRef)

		logBalancesTrieCreation(f.field, "fork (base from owned)")

		forked.base = base
		forked.overrides = make([]map[uint64][32]byte, depth+1)
		forked.updateMetrics()
		forked.cleanup = runtime.AddCleanup(forked, cleanupMetrics, forked.metrics)

		return forked
	}

	// Overlay mode: deep-copy overrides, share the base.
	forked.base = f.base

	overrides := make([]map[uint64][32]byte, len(f.overrides))
	for i, valueByIdx := range f.overrides {
		if len(valueByIdx) == 0 {
			continue
		}

		overrides[i] = make(map[uint64][32]byte, len(valueByIdx))
		maps.Copy(overrides[i], valueByIdx)
	}

	forked.overrides = overrides
	forked.updateMetrics()
	forked.cleanup = runtime.AddCleanup(forked, cleanupMetrics, forked.metrics)

	return forked
}

// recomputeInPlace performs the trie recomputation on the current trie..
func (f *FieldTrie) recomputeInPlace(indices []uint64, elements any) ([32]byte, error) {
	// Empty trie: rebuild from scratch.
	if f.empty() {
		root, err := f.rebuild(elements)
		if err != nil {
			return [32]byte{}, fmt.Errorf("rebuild empty trie: %w", err)
		}

		return root, nil
	}

	// Overlay with too many dirty leaves: rebuild from scratch and
	// cancel the pending overlay metric decrement.
	if f.base != nil && len(indices) > overlayPromotionThreshold {
		f.stopCleanup()
		FieldTriePromotionCounter.WithLabelValues(f.field.String()).Inc()

		root, err := f.rebuild(elements)
		if err != nil {
			return [32]byte{}, fmt.Errorf("rebuild overlay trie: %w", err)
		}

		return root, nil
	}

	// If no changes, return existing root.
	if len(indices) == 0 {
		return f.trieRoot()
	}

	if err := f.validateIndices(indices); err != nil {
		return [32]byte{}, fmt.Errorf("validate indices: %w", err)
	}

	// Owned: Only update affected branches in place.
	if f.base == nil {
		root, err := f.recomputeBranches(elements, indices)
		if err != nil {
			return [32]byte{}, fmt.Errorf("recompute owned: %w", err)
		}

		return root, nil
	}

	// Promote when the accumulated leaf-level overrides exceed the threshold.
	if len(f.overrides[0]) > overlayPromotionThreshold {
		root, err := f.promoteOverlay(elements, indices)
		if err != nil {
			return [32]byte{}, fmt.Errorf("promote overlay: %w", err)
		}

		return root, nil
	}

	root, err := f.recomputeOverlay(elements, indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("recompute overlay: %w", err)
	}

	return root, nil
}

// rebuild replaces the trie contents by building a fresh trie from elements.
func (f *FieldTrie) rebuild(elements any) ([32]byte, error) {
	nodes, offsets, err := buildTrie(f.field, elements, f.length)
	if err != nil {
		return [32]byte{}, fmt.Errorf("build trie: %w", err)
	}

	f.nodes = nodes
	f.offsets = offsets
	f.base = nil
	f.overrides = nil
	f.numOfElems = elemCount(elements)

	f.updateMetrics()
	f.cleanup = runtime.AddCleanup(f, cleanupMetrics, f.metrics)

	logBalancesTrieCreation(f.field, "rebuild")

	root, err := f.trieRoot()
	if err != nil {
		return [32]byte{}, fmt.Errorf("trie root: %w", err)
	}

	return root, nil
}

func logBalancesTrieCreation(field types.FieldIndex, source string) {
	if field != types.Balances {
		return
	}
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	log.WithField("source", source).Infof("Balances FieldTrie created\n%s", buf[:n])
}

// cleanupRef is a GC cleanup callback that decrements a reference count.
func cleanupRef(ref *stateutil.Reference) {
	ref.MinusRef()
}

// stopCleanup cancels the pending GC cleanup and resets entry metrics
// so that the next updateMetrics call computes correct deltas.
// The instance count gauge is left unchanged — updateMetrics handles
// mode transitions.
func (f *FieldTrie) stopCleanup() {
	f.cleanup.Stop()

	if f.metrics != nil {
		label := f.field.String()
		fieldTrieEntriesGauge.WithLabelValues(label, "nodes").Sub(float64(f.metrics.nodes))
		fieldTrieEntriesGauge.WithLabelValues(label, "overrides").Sub(float64(f.metrics.overrides))
		fieldTrieLeafOverridesGauge.WithLabelValues(label).Sub(float64(f.metrics.leafOverrides))

		mode := "owned"
		if f.metrics.overlay {
			mode = "overlay"
		}
		fieldTrieCountGauge.WithLabelValues(label, mode).Dec()

		// Zero so subsequent updateMetrics computes correct deltas.
		f.metrics.nodes = 0
		f.metrics.overrides = 0
		f.metrics.leafOverrides = 0
		f.metrics.overlay = false
	}
}

// Length return the length of the whole field trie.
func (f *FieldTrie) Length() uint64 {
	return f.length
}

// Empty checks whether the underlying field trie is empty or not.
func (f *FieldTrie) Empty() bool {
	if f == nil {
		return true
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.empty()
}

func (f *FieldTrie) isShared() bool {
	return f.ref.Refs() > 1 || f.dataRef.Refs() > 0
}

func (f *FieldTrie) empty() bool {
	return f.nodes == nil && f.base == nil
}

// IsOverlay returns true if this trie operates in overlay mode,
// storing sparse diffs against an immutable base.
func (f *FieldTrie) IsOverlay() bool {
	if f == nil {
		return false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.base != nil
}

// InsertFlatLayers manually inserts flat trie data. This method
// bypasses the normal method of field computation, it is only
// meant to be used in tests.
func (f *FieldTrie) InsertFlatLayers(nodes [][32]byte, offsets []uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes = nodes
	f.offsets = offsets
}

// recomputeBranches recomputes the trie branches for the given changed indices
// and returns the new trie root.
// - elements must be the complete collection
// - indices must contain only the changed positions
func (f *FieldTrie) recomputeBranches(elements any, indices []uint64) ([32]byte, error) {
	f.numOfElems = elemCount(elements)

	indices, err := f.compressedIndicesToChunks(indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("compressed indices to chunks: %w", err)
	}

	fieldRoots, err := fieldConverters(f.field, elements, indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("field converters: %w", err)
	}

	hasher := hash.CustomSHA256Hasher()

	var root [32]byte
	for i, idx := range indices {
		f.ensureLeafCapacity(idx + 1)

		f.nodes[idx] = fieldRoots[i]
		root = f.recomputeBranch(idx, hasher)
	}

	f.updateMetrics()

	rootWithMixin, err := f.rootWithMixin(root)
	if err != nil {
		return [32]byte{}, fmt.Errorf("root with mixin: %w", err)
	}

	return rootWithMixin, nil
}

// promoteOverlay promotes an overlay trie into an owned trie, incorporating the given changes,
// and returns the new trie root.
// - elements must be the complete collection
// - indices must contain only the changed positions
func (f *FieldTrie) promoteOverlay(elements any, indices []uint64) ([32]byte, error) {
	f.numOfElems = elemCount(elements)
	depth := f.base.depth()

	indices, err := f.compressedIndicesToChunks(indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("compressed indices to chunks: %w", err)
	}

	fieldRoots, err := fieldConverters(f.field, elements, indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("field converters: %w", err)
	}

	// Determine the leaf count for the new buffer.
	leafCount, err := f.leafCount()
	if err != nil {
		return [32]byte{}, fmt.Errorf("leaf count: %w", err)
	}

	for _, idx := range indices {
		leafCount = max(leafCount, idx+1)
	}

	// Allocate fresh buffer.
	f.offsets = computeOffsets(depth, leafCount)
	f.nodes = make([][32]byte, f.offsets[depth+1])

	// Skip the base copy when all leaves are being rewritten.
	if uint64(len(indices)) < leafCount {
		// Copy base layers into the new buffer.
		baseCount := min(f.base.levelSize(0), leafCount)
		copy(f.nodes[:baseCount], f.base.nodes[:baseCount])

		// Apply any existing overrides on top of the base copy.
		for idx, val := range f.overrides[0] {
			f.nodes[idx] = val
		}
	}

	// Apply new field roots for changed indices.
	for i, idx := range indices {
		f.nodes[idx] = fieldRoots[i]
	}

	hashUpFromLeaves(f.nodes, f.offsets)

	// Release the base.
	f.base = nil
	f.overrides = nil
	f.stopCleanup()

	FieldTriePromotionCounter.WithLabelValues(f.field.String()).Inc()
	f.updateMetrics()
	f.cleanup = runtime.AddCleanup(f, cleanupMetrics, f.metrics)

	// Return root with appropriate mixin.
	rootWithMixin, err := f.rootWithMixin(f.nodes[f.offsets[depth]])
	if err != nil {
		return [32]byte{}, fmt.Errorf("root with mixin: %w", err)
	}

	return rootWithMixin, nil
}

// recomputeOverlay recomputes the overlay trie for the given changes
// and returns the new trie root.
// - elements must be the complete collection
// - indices must contain only the changed positions
func (f *FieldTrie) recomputeOverlay(elements any, indices []uint64) ([32]byte, error) {
	f.numOfElems = elemCount(elements)

	indices, err := f.compressedIndicesToChunks(indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("compressed indices to chunks: %w", err)
	}

	fieldRoots, err := fieldConverters(f.field, elements, indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("field converters: %w", err)
	}

	dirtyLeaves := make(map[uint64][32]byte, len(indices))
	for i, idx := range indices {
		dirtyLeaves[idx] = fieldRoots[i]
	}

	// Store dirty leaves in overrides[0].
	if f.overrides[0] == nil {
		f.overrides[0] = make(map[uint64][32]byte, len(dirtyLeaves))
	}
	maps.Copy(f.overrides[0], dirtyLeaves)

	// Walk up from level 0 to depth-1.
	currentDirty := dirtyLeaves
	depth := f.base.depth()
	hasher := hash.CustomSHA256Hasher()

	var combinedChunks [64]byte
	for level := range depth {
		parentDirty := make(map[uint64][32]byte, len(currentDirty)/2+1)
		for idx := range currentDirty {
			parentIdx := idx / 2
			if _, ok := parentDirty[parentIdx]; ok {
				continue
			}

			leftIdx := parentIdx * 2
			rightIdx := leftIdx + 1

			left, err := f.readOverlayNode(level, leftIdx)
			if err != nil {
				return [32]byte{}, fmt.Errorf("read left overlay node: %w", err)
			}

			right, err := f.readOverlayNode(level, rightIdx)
			if err != nil {
				return [32]byte{}, fmt.Errorf("read right overlay node: %w", err)
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

	f.updateMetrics()

	// The root is at overrides[depth][0], or fallback to base.
	root, err := f.readOverlayNode(depth, 0)
	if err != nil {
		return [32]byte{}, fmt.Errorf("read overlay root: %w", err)
	}

	rootWithMixin, err := f.rootWithMixin(root)
	if err != nil {
		return [32]byte{}, fmt.Errorf("root with mixin: %w", err)
	}

	return rootWithMixin, nil
}

// readOverlayNode reads a node from the overlay at (level, idx).
func (f *FieldTrie) readOverlayNode(level uint64, idx uint64) ([32]byte, error) {
	// First, check if there is an override for this node.
	if nodeByIdx := f.overrides[level]; nodeByIdx != nil {
		if root, ok := nodeByIdx[idx]; ok {
			return root, nil
		}
	}

	// If no override, read from base.
	levelSize := f.base.levelSize(level)
	if idx < levelSize {
		return f.base.nodes[f.base.offsets[level]+idx], nil
	}

	// If idx is out of bounds for the base, return zero hash.
	return trie.ZeroHashes[level], nil
}

// compressedIndicesToChunks converts element-level indices to unique
// chunk-level indices for CompressedArray fields.
// For non-CompressedArray fields, returns the indices unchanged.
func (f *FieldTrie) compressedIndicesToChunks(indices []uint64) ([]uint64, error) {
	if f.dataType != types.CompressedArray {
		return indices, nil
	}

	numOfElems, err := f.field.ElemsInChunk()
	if err != nil {
		return nil, fmt.Errorf("elems in chunk: %w", err)
	}
	seen := make(map[uint64]bool, len(indices))
	chunkIndices := make([]uint64, 0, len(indices))

	for _, idx := range indices {
		chunkIdx := idx / numOfElems
		if seen[chunkIdx] {
			continue
		}

		seen[chunkIdx] = true
		chunkIndices = append(chunkIndices, chunkIdx)
	}

	return chunkIndices, nil
}

// ensureLeafCapacity grows the flat trie buffer to accommodate at least minLeafCount leaves.
// The leaf count adds 10% headroom to amortize repeated growth.
func (f *FieldTrie) ensureLeafCapacity(minLeafCount uint64) {
	if minLeafCount <= f.levelSize(0) {
		return
	}

	extra := minLeafCount / 10
	if extra == 0 {
		extra = 1
	}
	minLeafCount += extra

	depth := f.depth()
	newOffsets := computeOffsets(depth, minLeafCount)
	newNodes := make([][32]byte, newOffsets[depth+1])

	for level := range depth + 1 {
		oldSize := f.offsets[level+1] - f.offsets[level]
		newSize := newOffsets[level+1] - newOffsets[level]

		if oldSize > 0 {
			copy(newNodes[newOffsets[level]:], f.nodes[f.offsets[level]:f.offsets[level]+oldSize])
		}

		// ZeroHashes[0] == [32]byte{}, already zero-filled by make.
		if level == 0 {
			continue
		}

		// Initialize new entries to ZeroHashes[level] (empty subtree root).
		for i := oldSize; i < newSize; i++ {
			newNodes[newOffsets[level]+i] = trie.ZeroHashes[level]
		}
	}

	f.nodes = newNodes
	f.offsets = newOffsets
}

// recomputeBranch walks from a leaf index up to the root, recomputing parent hashes,
// and returns the new root hash.
func (f *FieldTrie) recomputeBranch(idx uint64, hasher func([]byte) [32]byte) [32]byte {
	root := f.nodes[idx]
	currentIndex := idx
	var combinedChunks [64]byte

	for level := range f.depth() {
		isLeft := currentIndex%2 == 0
		neighborIdx := currentIndex ^ 1
		levelSize := f.offsets[level+1] - f.offsets[level]

		neighbor := trie.ZeroHashes[level]
		if neighborIdx < levelSize {
			neighbor = f.nodes[f.offsets[level]+neighborIdx]
		}

		left, right := root, neighbor
		if !isLeft {
			left, right = neighbor, root
		}

		copy(combinedChunks[:32], left[:])
		copy(combinedChunks[32:], right[:])

		root = hasher(combinedChunks[:])
		parentIdx := currentIndex / 2
		f.nodes[f.offsets[level+1]+parentIdx] = root
		currentIndex = parentIdx
	}

	return root
}

// rootWithMixin applies the appropriate length mixin based on data type.
func (f *FieldTrie) rootWithMixin(root [32]byte) ([32]byte, error) {
	switch f.dataType {
	case types.BasicArray:
		return root, nil

	case types.CompositeArray, types.CompressedArray:
		var lengthBuf [32]byte
		binary.LittleEndian.PutUint64(lengthBuf[:], f.numOfElems)
		return ssz.MixInLength(root, lengthBuf[:]), nil

	default:
		return [32]byte{}, fmt.Errorf("unrecognized data type in field map: %v", reflect.TypeFor[types.DataType]().Name())
	}
}

// leafCount returns the number of leaves needed for the current elements.
// For compressed arrays, this is the number of chunks (ceil(numOfElems / elemsPerChunk)).
// For other types, this equals numOfElems (one leaf per element).
func (f *FieldTrie) leafCount() (uint64, error) {
	if f.dataType != types.CompressedArray {
		return f.numOfElems, nil
	}

	elemsPerChunk, err := f.field.ElemsInChunk()
	if err != nil {
		return 0, fmt.Errorf("elems in chunk: %w", err)
	}

	return (f.numOfElems + elemsPerChunk - 1) / elemsPerChunk, nil
}

// depth returns the trie depth from the offsets table.
func (f *FieldTrie) depth() uint64 {
	return uint64(len(f.offsets) - 2)
}

// levelSize returns the number of nodes at the given level.
func (f *FieldTrie) levelSize(level uint64) uint64 {
	return f.offsets[level+1] - f.offsets[level]
}

// nodeSize returns the total number of node entries in the flat buffer.
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

// leafOverrideSize returns the number of entries in the leaf-level (level 0) override map.
func (f *FieldTrie) leafOverrideSize() int {
	if len(f.overrides) == 0 {
		return 0
	}
	return len(f.overrides[0])
}

// updateMetrics syncs the Prometheus gauges with the current trie state.
// On first call (metrics == nil), it allocates the metricsRef and increments the
// instance count gauge. On subsequent calls, it computes deltas from the previous
// snapshot and adjusts gauges accordingly.
func (f *FieldTrie) updateMetrics() {
	if f.metrics == nil {
		f.metrics = &metricsRef{field: f.field}
		mode := overlayMode(f.base != nil)
		fieldTrieCountGauge.WithLabelValues(f.field.String(), mode).Inc()
		f.syncEntryGauges()

		return
	}

	isOverlay := f.base != nil
	if f.metrics.overlay != isOverlay {
		label := f.field.String()

		oldMode := overlayMode(f.metrics.overlay)
		fieldTrieCountGauge.WithLabelValues(label, oldMode).Dec()

		newMode := overlayMode(isOverlay)
		fieldTrieCountGauge.WithLabelValues(label, newMode).Inc()
	}

	f.syncEntryGauges()
}

// syncEntryGauges applies entry deltas to Prometheus gauges and updates the metricsRef snapshot.
func (f *FieldTrie) syncEntryGauges() {
	label := f.field.String()

	nodes := f.nodeSize()
	overrides := f.overlaySize()
	leafOverrides := f.leafOverrideSize()

	fieldTrieEntriesGauge.WithLabelValues(label, "nodes").Add(float64(nodes - f.metrics.nodes))
	fieldTrieEntriesGauge.WithLabelValues(label, "overrides").Add(float64(overrides - f.metrics.overrides))
	fieldTrieLeafOverridesGauge.WithLabelValues(label).Add(float64(leafOverrides - f.metrics.leafOverrides))

	f.metrics.nodes = nodes
	f.metrics.overrides = overrides
	f.metrics.leafOverrides = leafOverrides
	f.metrics.overlay = f.base != nil
}

// cleanupMetrics is the GC cleanup callback registered via runtime.AddCleanup.
// It subtracts the trie's metric contributions when the FieldTrie is garbage collected.
func cleanupMetrics(m *metricsRef) {
	label := m.field.String()

	fieldTrieEntriesGauge.WithLabelValues(label, "nodes").Sub(float64(m.nodes))
	fieldTrieEntriesGauge.WithLabelValues(label, "overrides").Sub(float64(m.overrides))
	fieldTrieLeafOverridesGauge.WithLabelValues(label).Sub(float64(m.leafOverrides))

	mode := overlayMode(m.overlay)
	fieldTrieCountGauge.WithLabelValues(label, mode).Dec()
}
