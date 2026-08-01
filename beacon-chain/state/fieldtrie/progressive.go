package fieldtrie

import (
	"fmt"
	"maps"
	"runtime"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/sirupsen/logrus"
)

type (
	progressiveNodesData struct {
		subtrees []*progressiveSubtreeData
		spine    [][32]byte
		metrics  *entriesMetric
	}

	progressiveSubtreeData struct {
		nodes   [][32]byte
		offsets []uint64
	}

	progressiveNodePosition struct {
		subtree int
		level   uint64
		index   uint64
	}

	progressiveOverridesData struct {
		nodes   map[progressiveNodePosition][32]byte
		spine   map[int][32]byte
		leaves  map[uint64]struct{}
		metrics *entriesMetric
	}
)

func buildProgressiveTrie(field types.FieldIndex, elements any) (*progressiveNodesData, error) {
	if elements == nil {
		return nil, nil
	}

	fieldRoots, err := fieldConverters(field, elements, nil)
	if err != nil {
		return nil, fmt.Errorf("field converters: %w", err)
	}

	numLevels := progressiveNumLevels(uint64(len(fieldRoots)))
	data := &progressiveNodesData{
		subtrees: make([]*progressiveSubtreeData, numLevels),
		spine:    make([][32]byte, numLevels),
	}

	for subtreeIndex := range numLevels {
		start := progressiveSubtreeStart(subtreeIndex)
		capacity := progressiveSubtreeCapacity(subtreeIndex)
		count := min(capacity, uint64(len(fieldRoots))-start)
		data.subtrees[subtreeIndex] = buildProgressiveSubtree(
			fieldRoots[start:start+count],
			progressiveSubtreeDepth(subtreeIndex),
		)
	}
	data.recomputeSpineFrom(numLevels - 1)
	return data, nil
}

func buildProgressiveSubtree(leaves [][32]byte, depth uint64) *progressiveSubtreeData {
	offsets := computeOffsets(depth, uint64(len(leaves)))
	nodes := make([][32]byte, offsets[depth+1])
	copy(nodes, leaves)
	hashUpFromLeaves(nodes, offsets)
	return &progressiveSubtreeData{nodes: nodes, offsets: offsets}
}

func (f *FieldTrie) progressiveTrieRoot() ([32]byte, error) {
	var root [32]byte
	if f.base == nil {
		if f.progressiveData == nil {
			return [32]byte{}, ErrInvalidFieldTrie
		}
		root = f.progressiveData.root()
	} else {
		root = f.readProgressiveOverlaySpine(0)
	}

	rootWithMixin, err := f.rootWithMixin(root)
	if err != nil {
		return [32]byte{}, fmt.Errorf("root with mixin: %w", err)
	}
	return rootWithMixin, nil
}

func (f *FieldTrie) recomputeProgressiveInPlace(indices []uint64, elements any) ([32]byte, error) {
	indiceCount := len(indices)
	promote := f.base != nil && indiceCount > f.promotionThreshold
	if promote {
		log.WithFields(logrus.Fields{
			"field":       f.field,
			"indiceCount": indiceCount,
			"threshold":   f.promotionThreshold,
		}).Debug("Promoting progressive overlay to owned")
		fieldTriePromotionCounter.WithLabelValues(f.field.String()).Inc()
	}

	if indices == nil || f.empty() || promote {
		return f.rebuildProgressiveFromScratch(elements)
	}
	if err := f.validateIndices(indices); err != nil {
		return [32]byte{}, fmt.Errorf("validate indices: %w", err)
	}
	if f.base == nil {
		return f.recomputeProgressiveOwned(elements, indices)
	}

	if len(f.progressiveOverridesData.leaves) > f.promotionThreshold {
		log.WithFields(logrus.Fields{
			"field":     f.field,
			"leafCount": len(f.progressiveOverridesData.leaves),
			"threshold": f.promotionThreshold,
		}).Debug("Promoting progressive overlay to owned")
		fieldTriePromotionCounter.WithLabelValues(f.field.String()).Inc()
		return f.rebuildProgressiveFromScratch(elements)
	}

	return f.recomputeProgressiveOverlay(elements, indices)
}

func (f *FieldTrie) rebuildProgressiveFromScratch(elements any) ([32]byte, error) {
	data, err := buildProgressiveTrie(f.field, elements)
	if err != nil {
		return [32]byte{}, fmt.Errorf("build progressive trie: %w", err)
	}

	f.releaseBase()
	f.base = nil
	f.nodesData = nil
	f.overridesData = nil
	f.progressiveOverridesData = nil
	f.progressiveData = nil
	f.numOfElems = elemCount(elements)
	if data != nil {
		f.progressiveData = newProgressiveNodesData(f.field, data)
	}

	root, err := f.trieRoot()
	if err != nil {
		return [32]byte{}, fmt.Errorf("trie root: %w", err)
	}
	return root, nil
}

func (f *FieldTrie) recomputeProgressiveOwned(elements any, indices []uint64) ([32]byte, error) {
	f.numOfElems = elemCount(elements)

	chunkIndices, err := f.compressedIndicesToChunks(indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("compressed indices to chunks: %w", err)
	}
	fieldRoots, err := fieldConverters(f.field, elements, chunkIndices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("field converters: %w", err)
	}

	for i, globalIndex := range chunkIndices {
		subtreeIndex, localIndex := progressiveSubtreeForIndex(globalIndex)
		f.progressiveData.ensureLeafCapacity(subtreeIndex, localIndex+1)
		subtree := f.progressiveData.subtrees[subtreeIndex]
		subtree.nodes[subtree.offsets[0]+localIndex] = fieldRoots[i]
		subtree.recomputeBranch(localIndex)
		f.progressiveData.recomputeSpineFrom(subtreeIndex)
	}
	f.progressiveData.updateMetrics()

	rootWithMixin, err := f.rootWithMixin(f.progressiveData.root())
	if err != nil {
		return [32]byte{}, fmt.Errorf("root with mixin: %w", err)
	}
	return rootWithMixin, nil
}

func (f *FieldTrie) recomputeProgressiveOverlay(elements any, indices []uint64) ([32]byte, error) {
	f.numOfElems = elemCount(elements)

	chunkIndices, err := f.compressedIndicesToChunks(indices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("compressed indices to chunks: %w", err)
	}
	fieldRoots, err := fieldConverters(f.field, elements, chunkIndices)
	if err != nil {
		return [32]byte{}, fmt.Errorf("field converters: %w", err)
	}

	for i, globalIndex := range chunkIndices {
		subtreeIndex, localIndex := progressiveSubtreeForIndex(globalIndex)
		depth := progressiveSubtreeDepth(subtreeIndex)
		position := progressiveNodePosition{subtree: subtreeIndex, level: 0, index: localIndex}
		f.progressiveOverridesData.nodes[position] = fieldRoots[i]
		f.progressiveOverridesData.leaves[globalIndex] = struct{}{}

		currentIndex := localIndex
		var pair [64]byte
		hasher := hash.CustomSHA256Hasher()
		for level := range depth {
			parentIndex := currentIndex / 2
			leftIndex := parentIndex * 2
			left := f.readProgressiveOverlayNode(subtreeIndex, level, leftIndex)
			right := f.readProgressiveOverlayNode(subtreeIndex, level, leftIndex+1)
			copy(pair[:32], left[:])
			copy(pair[32:], right[:])
			parent := hasher(pair[:])
			currentIndex = parentIndex
			f.progressiveOverridesData.nodes[progressiveNodePosition{
				subtree: subtreeIndex,
				level:   level + 1,
				index:   parentIndex,
			}] = parent
		}

		f.recomputeProgressiveOverlaySpine(subtreeIndex)
	}
	f.progressiveOverridesData.updateMetrics()

	rootWithMixin, err := f.rootWithMixin(f.readProgressiveOverlaySpine(0))
	if err != nil {
		return [32]byte{}, fmt.Errorf("root with mixin: %w", err)
	}
	return rootWithMixin, nil
}

func (f *FieldTrie) recomputeProgressiveOverlaySpine(from int) {
	successor := f.readProgressiveOverlaySpine(from + 1)
	hasher := hash.CustomSHA256Hasher()
	var pair [64]byte

	for subtreeIndex := from; subtreeIndex >= 0; subtreeIndex-- {
		depth := progressiveSubtreeDepth(subtreeIndex)
		subtreeRoot := f.readProgressiveOverlayNode(subtreeIndex, depth, 0)
		copy(pair[:32], subtreeRoot[:])
		copy(pair[32:], successor[:])
		successor = hasher(pair[:])
		f.progressiveOverridesData.spine[subtreeIndex] = successor
	}
}

func (f *FieldTrie) readProgressiveOverlayNode(subtree int, level, index uint64) [32]byte {
	position := progressiveNodePosition{subtree: subtree, level: level, index: index}
	if root, ok := f.progressiveOverridesData.nodes[position]; ok {
		return root
	}
	return f.base.progressiveData.readNode(subtree, level, index)
}

func (f *FieldTrie) readProgressiveOverlaySpine(level int) [32]byte {
	if level < 0 {
		return [32]byte{}
	}
	if root, ok := f.progressiveOverridesData.spine[level]; ok {
		return root
	}
	if level < len(f.base.progressiveData.spine) {
		return f.base.progressiveData.spine[level]
	}
	return [32]byte{}
}

func (p *progressiveNodesData) root() [32]byte {
	if p == nil || len(p.spine) == 0 {
		return [32]byte{}
	}
	return p.spine[0]
}

func (p *progressiveNodesData) readNode(subtree int, level, index uint64) [32]byte {
	if subtree < 0 || subtree >= len(p.subtrees) || p.subtrees[subtree] == nil {
		return trie.ZeroHashes[level]
	}
	data := p.subtrees[subtree]
	if level+1 >= uint64(len(data.offsets)) {
		return trie.ZeroHashes[level]
	}
	levelSize := data.offsets[level+1] - data.offsets[level]
	if index >= levelSize {
		return trie.ZeroHashes[level]
	}
	return data.nodes[data.offsets[level]+index]
}

func (p *progressiveNodesData) recomputeSpineFrom(from int) {
	if from < 0 {
		return
	}
	if len(p.spine) <= from {
		p.spine = append(p.spine, make([][32]byte, from-len(p.spine)+1)...)
	}

	successor := [32]byte{}
	if from+1 < len(p.spine) {
		successor = p.spine[from+1]
	}
	hasher := hash.CustomSHA256Hasher()
	var pair [64]byte
	for subtreeIndex := from; subtreeIndex >= 0; subtreeIndex-- {
		depth := progressiveSubtreeDepth(subtreeIndex)
		subtreeRoot := p.readNode(subtreeIndex, depth, 0)
		copy(pair[:32], subtreeRoot[:])
		copy(pair[32:], successor[:])
		successor = hasher(pair[:])
		p.spine[subtreeIndex] = successor
	}
}

func (p *progressiveNodesData) ensureLeafCapacity(subtreeIndex int, minLeafCount uint64) {
	if len(p.subtrees) <= subtreeIndex {
		p.subtrees = append(p.subtrees, make([]*progressiveSubtreeData, subtreeIndex-len(p.subtrees)+1)...)
	}

	capacity := progressiveSubtreeCapacity(subtreeIndex)
	minLeafCount = min(minLeafCount, capacity)
	subtree := p.subtrees[subtreeIndex]
	if subtree != nil && minLeafCount <= subtree.levelSize(0) {
		return
	}

	allocatedLeafCount := minLeafCount
	extra := minLeafCount / 10
	if extra == 0 {
		extra = 1
	}
	allocatedLeafCount = min(capacity, allocatedLeafCount+extra)
	depth := progressiveSubtreeDepth(subtreeIndex)
	newOffsets := computeOffsets(depth, allocatedLeafCount)
	newNodes := make([][32]byte, newOffsets[depth+1])

	if subtree != nil {
		for level := uint64(0); level <= depth; level++ {
			oldSize := subtree.levelSize(level)
			copy(
				newNodes[newOffsets[level]:newOffsets[level]+oldSize],
				subtree.nodes[subtree.offsets[level]:subtree.offsets[level]+oldSize],
			)
		}
	}

	for level := uint64(1); level <= depth; level++ {
		oldSize := uint64(0)
		if subtree != nil {
			oldSize = subtree.levelSize(level)
		}
		newSize := newOffsets[level+1] - newOffsets[level]
		for index := oldSize; index < newSize; index++ {
			newNodes[newOffsets[level]+index] = trie.ZeroHashes[level]
		}
	}

	p.subtrees[subtreeIndex] = &progressiveSubtreeData{nodes: newNodes, offsets: newOffsets}
	if len(p.spine) <= subtreeIndex {
		p.spine = append(p.spine, make([][32]byte, subtreeIndex-len(p.spine)+1)...)
	}
}

func (p *progressiveNodesData) entryCount() int {
	if p == nil {
		return 0
	}
	count := len(p.spine)
	for _, subtree := range p.subtrees {
		if subtree != nil {
			count += len(subtree.nodes)
		}
	}
	return count
}

func (p *progressiveNodesData) updateMetrics() {
	if p == nil || p.metrics == nil {
		return
	}
	newCount := p.entryCount()
	fieldTrieEntriesGauge.WithLabelValues(p.metrics.field.String(), "nodes").Add(float64(newCount - p.metrics.totalCount))
	p.metrics.totalCount = newCount
}

func (p *progressiveSubtreeData) levelSize(level uint64) uint64 {
	return p.offsets[level+1] - p.offsets[level]
}

func (p *progressiveSubtreeData) recomputeBranch(index uint64) {
	root := p.nodes[p.offsets[0]+index]
	currentIndex := index
	hasher := hash.CustomSHA256Hasher()
	var pair [64]byte

	depth := uint64(len(p.offsets) - 2)
	for level := range depth {
		neighborIndex := currentIndex ^ 1
		neighbor := trie.ZeroHashes[level]
		if neighborIndex < p.levelSize(level) {
			neighbor = p.nodes[p.offsets[level]+neighborIndex]
		}
		left, right := root, neighbor
		if currentIndex%2 == 1 {
			left, right = neighbor, root
		}
		copy(pair[:32], left[:])
		copy(pair[32:], right[:])
		root = hasher(pair[:])
		currentIndex /= 2
		p.nodes[p.offsets[level+1]+currentIndex] = root
	}
}

func newProgressiveNodesData(field types.FieldIndex, data *progressiveNodesData) *progressiveNodesData {
	count := data.entryCount()
	data.metrics = &entriesMetric{field: field, totalCount: count}
	fieldTrieEntriesGauge.WithLabelValues(field.String(), "nodes").Add(float64(count))
	fieldTrieCountGauge.WithLabelValues(field.String(), string(trieModeOwned)).Inc()
	runtime.AddCleanup(data, cleanupNodesMetrics, data.metrics)
	return data
}

func newProgressiveOverridesData(field types.FieldIndex) *progressiveOverridesData {
	data := &progressiveOverridesData{
		nodes:   make(map[progressiveNodePosition][32]byte),
		spine:   make(map[int][32]byte),
		leaves:  make(map[uint64]struct{}),
		metrics: &entriesMetric{field: field},
	}
	fieldTrieCountGauge.WithLabelValues(field.String(), string(trieModeOverlay)).Inc()
	runtime.AddCleanup(data, cleanupOverridesMetrics, data.metrics)
	return data
}

func (p *progressiveOverridesData) copy(field types.FieldIndex) *progressiveOverridesData {
	copied := newProgressiveOverridesData(field)
	maps.Copy(copied.nodes, p.nodes)
	maps.Copy(copied.spine, p.spine)
	maps.Copy(copied.leaves, p.leaves)
	copied.updateMetrics()
	return copied
}

func (p *progressiveOverridesData) updateMetrics() {
	newCount := len(p.nodes) + len(p.spine)
	newLeafCount := len(p.leaves)
	label := p.metrics.field.String()
	fieldTrieEntriesGauge.WithLabelValues(label, "overrides").Add(float64(newCount - p.metrics.totalCount))
	fieldTrieLeafOverridesGauge.WithLabelValues(label).Add(float64(newLeafCount - p.metrics.leafCount))
	p.metrics.totalCount = newCount
	p.metrics.leafCount = newLeafCount
}

func progressiveSubtreeCapacity(level int) uint64 {
	return uint64(1) << (2 * level)
}

func progressiveSubtreeDepth(level int) uint64 {
	return uint64(2 * level)
}

func progressiveSubtreeStart(level int) uint64 {
	var start uint64
	for i := range level {
		start += progressiveSubtreeCapacity(i)
	}
	return start
}

func progressiveNumLevels(numLeaves uint64) int {
	levels := 0
	var capacity uint64
	for capacity < numLeaves {
		capacity += progressiveSubtreeCapacity(levels)
		levels++
	}
	return levels
}

func progressiveSubtreeForIndex(globalIndex uint64) (int, uint64) {
	level := progressiveNumLevels(globalIndex+1) - 1
	return level, globalIndex - progressiveSubtreeStart(level)
}
