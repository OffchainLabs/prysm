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
	"github.com/sirupsen/logrus"
)

var (
	ErrInvalidFieldTrie = errors.New("invalid field trie")
	ErrEmptyFieldTrie   = errors.New("empty field trie")
)

// defaultPromotionThreshold is the maximum number of dirty leaves
// before an overlay is promoted to a full trie rebuild.
//
// The overlay path costs O(k × depth) with per-entry map overhead,
// where k is the number of dirty leaves.
// The rebuild path costs O(n) with vectorized hashing, where n is
// the total number of leaves.
// At ~20K dirty leaves and depth ~40, the overlay's map-heavy random
// access starts to exceed the cost of a flat sequential rebuild over
// ~2M leaves.
const defaultPromotionThreshold = 20_000

type (
	// FieldTrie is the representation of the representative
	// trie of the particular field.
	FieldTrie struct {
		mu sync.RWMutex

		ref *stateutil.Reference // count of holders pointing to this FieldTrie

		dataRef        *stateutil.Reference // count of overlay bases sharing this trie's nodes buffer
		dataRefCleanup runtime.Cleanup      // cleanup callback for dataRef

		// Owned mode (nil in overlay mode):
		nodesData *nodesData // data relative to the nodes of the trie

		// Overlay mode (nil in owned mode):
		base          *FieldTrie     // immutable base trie
		overridesData *overridesData // per-level sparse diffs

		// Field metadata:
		field              types.FieldIndex // which beacon state field this trie represents
		dataType           types.DataType   // encoding: BasicArray, CompositeArray, or CompressedArray
		length             uint64           // maximum capacity
		numOfElems         uint64           // current number of elems in the field
		promotionThreshold int              // resolved promotion threshold
	}

	nodesData struct {
		nodes   [][32]byte     // flat buffer with all trie levels packed contiguously
		offsets []uint64       // maps each trie level to its start index in nodes. Also offsets[depth+1] = len(nodes)
		metrics *entriesMetric // tracks the number of node entries for metrics purposes
	}

	overridesData struct {
		levels  []map[uint64][32]byte // per-level sparse diffs: levels[level][nodeIdx] = hash
		metrics *entriesMetric        // tracks the number of override entries and leaf override entries for metrics purposes
	}

	entriesMetric struct {
		field      types.FieldIndex // which field this metric is tracking
		totalCount int              // total entries (nodes or override entries)
		leafCount  int              // leaf-level entries
	}

	// sliceAccessor describes an interface for a multivalue slice
	// object that returns information about the multivalue slice along with the
	// particular state instance we are referencing.
	sliceAccessor interface {
		Len(obj multi_value_slice.Identifiable) int
		State() multi_value_slice.Identifiable
	}
)

// NewFieldTrie creates a new field trie from the given elements.
// length is the maximum capacity of the field and determines
// the trie depth. The number of elements must be <= length.
// promotionThreshold, when > 0, overrides the defaultPromotionThreshold with an absolute count.
// When 0, defaultPromotionThreshold is used.
func NewFieldTrie(field types.FieldIndex, fieldInfo types.DataType, elements any, length uint64, promotionThreshold int) (*FieldTrie, error) {
	if !map[types.DataType]bool{
		types.BasicArray:      true,
		types.CompositeArray:  true,
		types.CompressedArray: true,
	}[fieldInfo] {
		return nil, errors.Errorf("unrecognized data type in field map: %v", reflect.TypeFor[types.DataType]().Name())
	}

	if promotionThreshold <= 0 {
		promotionThreshold = defaultPromotionThreshold
	}

	if err := validateElements(field, fieldInfo, elements, length); err != nil {
		return nil, fmt.Errorf("validate elements: %w", err)
	}

	nodes, offsets, err := buildTrie(field, elements, length)
	if err != nil {
		return nil, fmt.Errorf("build trie: %w", err)
	}

	fieldTrie := &FieldTrie{
		ref:                stateutil.NewRef(1),
		dataRef:            stateutil.NewRef(0),
		field:              field,
		dataType:           fieldInfo,
		length:             length,
		numOfElems:         elemCount(elements),
		promotionThreshold: promotionThreshold,
	}

	runtime.AddCleanup(fieldTrie, cleanupRef, fieldTrie.ref)

	if nodes != nil {
		fieldTrie.nodesData = newNodesData(field, nodes, offsets)
	}

	return fieldTrie, nil
}

// CopyTrie creates a lightweight copy that shares the underlying trie data.
func (f *FieldTrie) CopyTrie() *FieldTrie {
	f.mu.RLock()
	defer f.mu.RUnlock()

	mode := string(overlayMode(f.base != nil))
	fieldTrieCopyCounter.WithLabelValues(f.field.String(), mode).Inc()

	f.ref.AddRef()

	copiedTrie := &FieldTrie{
		ref:                f.ref,
		dataRef:            f.dataRef,
		nodesData:          f.nodesData,
		base:               f.base,
		overridesData:      f.overridesData,
		field:              f.field,
		dataType:           f.dataType,
		length:             f.length,
		numOfElems:         f.numOfElems,
		promotionThreshold: f.promotionThreshold,
	}

	if f.base != nil {
		f.base.dataRef.AddRef()
		copiedTrie.dataRefCleanup = runtime.AddCleanup(copiedTrie, cleanupRef, f.base.dataRef)
	}

	runtime.AddCleanup(copiedTrie, cleanupRef, f.ref)

	return copiedTrie
}

// TrieRoot returns the root of the trie with the appropriate length mixin applied.
func (f *FieldTrie) TrieRoot() ([32]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.trieRoot()
}

// RecomputeTrie recomputes the trie for the given changed indices and returns
// the new trie and root hash. When indices is nil, the trie is rebuilt from
// scratch using elements. The caller MUST use the returned *FieldTrie
// in place of the one on which this method was called with, even if an error
// is returned.
func (f *FieldTrie) RecomputeTrie(indices []uint64, elements any) (*FieldTrie, [32]byte, error) {
	if indices != nil {
		// Deduplicating indices to avoid redundant recomputation.
		indices = slice.SetUint64(indices)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	fieldTrieRecomputeIndicesSummary.WithLabelValues(f.field.String()).Observe(float64(len(indices)))

	// If no changes, return existing root (read-only).
	if !f.empty() && indices != nil && len(indices) == 0 {
		root, err := f.trieRoot()
		return f, root, err
	}

	// If field is shared, snapshot source data under the lock, then recompute on the fork.
	if f.isShared() {
		forked := f.fork()

		root, err := forked.recomputeInPlace(indices, elements)
		if err != nil {
			return forked, [32]byte{}, fmt.Errorf("recompute in place after fork: %w", err)
		}

		return forked, root, nil
	}

	// If field is not shared, recompute in place.
	root, err := f.recomputeInPlace(indices, elements)
	if err != nil {
		return f, [32]byte{}, fmt.Errorf("recompute in place: %w", err)
	}

	return f, root, nil
}

// Empty checks whether the underlying field trie is empty or not.
// It is only meant to be used in tests.
func (f *FieldTrie) Empty() bool {
	if f == nil {
		return true
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.empty()
}

// RefCount returns the reference count of this field trie, which indicates how many
// BeaconState instances are sharing this trie.
// A count of 1 means no sharing, and >1 means shared.
func (f *FieldTrie) RefCount() uint {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.ref.Refs()
}

// InsertFieldLayer manually inserts flat trie data. This method
// bypasses the normal method of field computation, it is only
// meant to be used in tests.
func (f *FieldTrie) InsertFieldLayer(nodes [][32]byte, offsets []uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nodesData = &nodesData{nodes: nodes, offsets: offsets}
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

		rootOffset := f.nodesData.offsets[depth]
		root := f.nodesData.nodes[rootOffset]
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
	fieldTrieForkCounter.WithLabelValues(f.field.String(), string(overlayMode(f.base != nil))).Inc()

	forked := &FieldTrie{
		ref:                stateutil.NewRef(1),
		dataRef:            stateutil.NewRef(0),
		field:              f.field,
		dataType:           f.dataType,
		length:             f.length,
		numOfElems:         f.numOfElems,
		promotionThreshold: f.promotionThreshold,
	}

	runtime.AddCleanup(forked, cleanupRef, forked.ref)

	if f.empty() {
		return forked
	}

	// Owned mode: use source directly as immutable base for the fork.
	if f.base == nil {
		f.dataRef.AddRef()
		forked.base = f

		forked.dataRefCleanup = runtime.AddCleanup(forked, cleanupRef, f.dataRef)
		forked.overridesData = newOverridesData(f.field, make([]map[uint64][32]byte, f.depth()+1))

		return forked
	}

	// Overlay mode: share the base and deep-copy overrides.
	forked.base = f.base
	f.base.dataRef.AddRef()
	forked.dataRefCleanup = runtime.AddCleanup(forked, cleanupRef, f.base.dataRef)

	levels := make([]map[uint64][32]byte, len(f.overridesData.levels))
	for i, valueByIdx := range f.overridesData.levels {
		if len(valueByIdx) == 0 {
			continue
		}

		levels[i] = make(map[uint64][32]byte, len(valueByIdx))
		maps.Copy(levels[i], valueByIdx)
	}

	forked.overridesData = newOverridesData(f.field, levels)

	return forked
}

// recomputeInPlace performs the trie recomputation on the current trie.
func (f *FieldTrie) recomputeInPlace(indices []uint64, elements any) ([32]byte, error) {
	indiceCount := len(indices)
	promote := f.base != nil && indiceCount > f.promotionThreshold
	if promote {
		log.WithFields(logrus.Fields{
			"field":       f.field,
			"indiceCount": indiceCount,
			"threshold":   f.promotionThreshold,
		}).Debug("Promoting overlay to owned")

		fieldTriePromotionCounter.WithLabelValues(f.field.String()).Inc()
	}

	// Owned or overlay: Rebuild from scratch if no indices provided,
	// field is empty, or promotion threshold exceeded.
	if indices == nil || f.empty() || promote {
		root, err := f.rebuildFromScratch(elements)
		if err != nil {
			return [32]byte{}, fmt.Errorf("rebuild from scratch: %w", err)
		}

		return root, nil
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

	// Overlay: Promote when the accumulated leaf-level overrides exceed the threshold.
	leafCount := len(f.overridesData.levels[0])
	if leafCount > f.promotionThreshold {
		log.WithFields(logrus.Fields{
			"field":     f.field,
			"leafCount": leafCount,
			"threshold": f.promotionThreshold,
		}).Debug("Promoting overlay to owned")

		root, err := f.promoteOverlay(elements, indices)
		if err != nil {
			return [32]byte{}, fmt.Errorf("promote overlay: %w", err)
		}

		return root, nil
	}

	// Overlay: Update overrides and recompute affected overlay branches.
	root, err := f.recomputeOverlay(elements, indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("recompute overlay: %w", err)
	}

	return root, nil
}

// rebuild replaces the trie contents by building a fresh trie from elements.
func (f *FieldTrie) rebuildFromScratch(elements any) ([32]byte, error) {
	nodes, offsets, err := buildTrie(f.field, elements, f.length)
	if err != nil {
		return [32]byte{}, fmt.Errorf("build trie: %w", err)
	}

	f.releaseBase()

	f.base = nil
	f.overridesData = nil
	f.numOfElems = elemCount(elements)

	f.nodesData = nil
	if nodes != nil {
		f.nodesData = newNodesData(f.field, nodes, offsets)
	}

	root, err := f.trieRoot()
	if err != nil {
		return [32]byte{}, fmt.Errorf("trie root: %w", err)
	}

	return root, nil
}

func (f *FieldTrie) releaseBase() {
	if f.base == nil {
		return
	}

	f.dataRefCleanup.Stop()
	f.base.dataRef.MinusRef()
}

// cleanupRef is a GC cleanup callback that decrements a reference count.
func cleanupRef(ref *stateutil.Reference) {
	ref.MinusRef()
}

func (f *FieldTrie) isShared() bool {
	return f.ref.Refs() > 1 || f.dataRef.Refs() > 0
}

func (f *FieldTrie) empty() bool {
	return f.nodesData == nil && f.base == nil
}

// recomputeBranches recomputes the trie branches for the given changed indices
// and returns the new trie root.
// - elements must be the complete collection
// - indices must contain only the changed positions
func (f *FieldTrie) recomputeBranches(elements any, indices []uint64) ([32]byte, error) {
	f.numOfElems = elemCount(elements)

	chunkIndices, err := f.compressedIndicesToChunks(indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("compressed indices to chunks: %w", err)
	}

	fieldRoots, err := fieldConverters(f.field, elements, chunkIndices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("field converters: %w", err)
	}

	hasher := hash.CustomSHA256Hasher()

	var root [32]byte
	for i, idx := range chunkIndices {
		f.ensureLeafCapacity(idx + 1)

		f.nodesData.nodes[idx] = fieldRoots[i]
		root = f.recomputeBranch(idx, hasher)
	}

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

	chunkIndices, err := f.compressedIndicesToChunks(indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("compressed indices to chunks: %w", err)
	}

	fieldRoots, err := fieldConverters(f.field, elements, chunkIndices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("field converters: %w", err)
	}

	// Determine the leaf count for the new buffer.
	leafCount, err := f.leafCount()
	if err != nil {
		return [32]byte{}, fmt.Errorf("leaf count: %w", err)
	}

	for _, idx := range chunkIndices {
		leafCount = max(leafCount, idx+1)
	}

	// Allocate fresh buffer.
	offsets := computeOffsets(depth, leafCount)
	nodes := make([][32]byte, offsets[depth+1])

	// Skip the base copy when all leaves are being rewritten.
	if uint64(len(chunkIndices)) < leafCount {
		// Copy base layers into the new buffer.
		baseCount := min(f.base.levelSize(0), leafCount)
		copy(nodes[:baseCount], f.base.nodesData.nodes[:baseCount])

		// Apply any existing overrides on top of the base copy.
		for idx, val := range f.overridesData.levels[0] {
			nodes[idx] = val
		}
	}

	// Apply new field roots for changed indices.
	for i, idx := range chunkIndices {
		nodes[idx] = fieldRoots[i]
	}

	hashUpFromLeaves(nodes, offsets)

	// Release the base.
	f.releaseBase()
	f.base = nil
	f.overridesData = nil

	f.nodesData = newNodesData(f.field, nodes, offsets)

	fieldTriePromotionCounter.WithLabelValues(f.field.String()).Inc()

	// Return root with appropriate mixin.
	rootWithMixin, err := f.rootWithMixin(nodes[offsets[depth]])
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

	chunkIndices, err := f.compressedIndicesToChunks(indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("compressed indices to chunks: %w", err)
	}

	fieldRoots, err := fieldConverters(f.field, elements, chunkIndices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("field converters: %w", err)
	}

	dirtyLeaves := make(map[uint64][32]byte, len(chunkIndices))
	for i, idx := range chunkIndices {
		dirtyLeaves[idx] = fieldRoots[i]
	}

	// Store dirty leaves in levels[0].
	if f.overridesData.levels[0] == nil {
		f.overridesData.levels[0] = make(map[uint64][32]byte, len(dirtyLeaves))
	}
	maps.Copy(f.overridesData.levels[0], dirtyLeaves)

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
			if f.overridesData.levels[level+1] == nil {
				f.overridesData.levels[level+1] = make(map[uint64][32]byte)
			}

			f.overridesData.levels[level+1][parentIdx] = parentHash
		}

		currentDirty = parentDirty
	}

	f.overridesData.updateMetrics()

	// The root is at levels[depth][0], or fallback to base.
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
	if nodeByIdx := f.overridesData.levels[level]; nodeByIdx != nil {
		if root, ok := nodeByIdx[idx]; ok {
			return root, nil
		}
	}

	// If no override, read from base.
	levelSize := f.base.levelSize(level)
	if idx < levelSize {
		return f.base.nodesData.nodes[f.base.nodesData.offsets[level]+idx], nil
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
		oldSize := f.nodesData.offsets[level+1] - f.nodesData.offsets[level]
		newSize := newOffsets[level+1] - newOffsets[level]

		if oldSize > 0 {
			copy(newNodes[newOffsets[level]:], f.nodesData.nodes[f.nodesData.offsets[level]:f.nodesData.offsets[level]+oldSize])
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

	f.nodesData.nodes = newNodes
	f.nodesData.offsets = newOffsets

	f.nodesData.updateMetrics()
}

// recomputeBranch walks from a leaf index up to the root, recomputing parent hashes,
// and returns the new root hash.
func (f *FieldTrie) recomputeBranch(idx uint64, hasher func([]byte) [32]byte) [32]byte {
	root := f.nodesData.nodes[idx]
	currentIndex := idx
	var combinedChunks [64]byte

	for level := range f.depth() {
		isLeft := currentIndex%2 == 0
		neighborIdx := currentIndex ^ 1
		levelSize := f.nodesData.offsets[level+1] - f.nodesData.offsets[level]

		neighbor := trie.ZeroHashes[level]
		if neighborIdx < levelSize {
			neighbor = f.nodesData.nodes[f.nodesData.offsets[level]+neighborIdx]
		}

		left, right := root, neighbor
		if !isLeft {
			left, right = neighbor, root
		}

		copy(combinedChunks[:32], left[:])
		copy(combinedChunks[32:], right[:])

		root = hasher(combinedChunks[:])
		parentIdx := currentIndex / 2
		f.nodesData.nodes[f.nodesData.offsets[level+1]+parentIdx] = root
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
	return uint64(len(f.nodesData.offsets) - 2)
}

// levelSize returns the number of nodes at the given level.
func (f *FieldTrie) levelSize(level uint64) uint64 {
	return f.nodesData.offsets[level+1] - f.nodesData.offsets[level]
}

// newNodesData allocates a nodesData, increments the node entry gauge,
// and registers a GC cleanup to decrement it when the nodesData is collected.
func newNodesData(field types.FieldIndex, nodes [][32]byte, offsets []uint64) *nodesData {
	nodesData := &nodesData{
		nodes:   nodes,
		offsets: offsets,
		metrics: &entriesMetric{field: field, totalCount: len(nodes)},
	}

	label := field.String()
	fieldTrieEntriesGauge.WithLabelValues(label, "nodes").Add(float64(len(nodes)))
	fieldTrieCountGauge.WithLabelValues(label, string(trieModeOwned)).Inc()
	runtime.AddCleanup(nodesData, cleanupNodesMetrics, nodesData.metrics)

	return nodesData
}

// updateMetrics applies the delta between the current nodes length and
// the last-recorded snapshot, then updates the snapshot.
func (nd *nodesData) updateMetrics() {
	if nd.metrics == nil {
		return
	}

	newCount := len(nd.nodes)
	fieldTrieEntriesGauge.WithLabelValues(nd.metrics.field.String(), "nodes").Add(float64(newCount - nd.metrics.totalCount))
	nd.metrics.totalCount = newCount
}

// newOverridesData allocates an overridesData, increments the override entry gauges,
// and registers a GC cleanup to decrement them when the overridesData is collected.
func newOverridesData(field types.FieldIndex, levels []map[uint64][32]byte) *overridesData {
	totalCount := 0
	for _, m := range levels {
		totalCount += len(m)
	}

	leafCount := 0
	if len(levels) > 0 {
		leafCount = len(levels[0])
	}

	od := &overridesData{
		levels:  levels,
		metrics: &entriesMetric{field: field, totalCount: totalCount, leafCount: leafCount},
	}

	label := field.String()
	fieldTrieEntriesGauge.WithLabelValues(label, "overrides").Add(float64(totalCount))
	fieldTrieLeafOverridesGauge.WithLabelValues(label).Add(float64(leafCount))
	fieldTrieCountGauge.WithLabelValues(label, string(trieModeOverlay)).Inc()

	runtime.AddCleanup(od, cleanupOverridesMetrics, od.metrics)

	return od
}

// updateMetrics applies deltas between the current override counts
// and the last-recorded snapshot, then updates the snapshot.
func (od *overridesData) updateMetrics() {
	newCount := 0
	for _, m := range od.levels {
		newCount += len(m)
	}

	newLeaf := 0
	if len(od.levels) > 0 {
		newLeaf = len(od.levels[0])
	}

	label := od.metrics.field.String()
	fieldTrieEntriesGauge.WithLabelValues(label, "overrides").Add(float64(newCount - od.metrics.totalCount))
	fieldTrieLeafOverridesGauge.WithLabelValues(label).Add(float64(newLeaf - od.metrics.leafCount))
	od.metrics.totalCount = newCount
	od.metrics.leafCount = newLeaf
}

// cleanupNodesMetrics is the GC cleanup callback that decrements node entry gauges
// when the nodesData becomes unreachable.
func cleanupNodesMetrics(met *entriesMetric) {
	label := met.field.String()
	fieldTrieEntriesGauge.WithLabelValues(label, "nodes").Sub(float64(met.totalCount))
	fieldTrieCountGauge.WithLabelValues(label, string(trieModeOwned)).Dec()
}

// cleanupOverridesMetrics is the GC cleanup callback that decrements override gauges
// when the overridesData becomes unreachable.
func cleanupOverridesMetrics(met *entriesMetric) {
	label := met.field.String()
	fieldTrieEntriesGauge.WithLabelValues(label, "overrides").Sub(float64(met.totalCount))
	fieldTrieLeafOverridesGauge.WithLabelValues(label).Sub(float64(met.leafCount))
	fieldTrieCountGauge.WithLabelValues(label, string(trieModeOverlay)).Dec()
}
