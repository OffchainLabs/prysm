package query

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"reflect"
	"runtime"
	"slices"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ssz "github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/math"
	fastssz "github.com/prysmaticlabs/fastssz"
)

// proofCollector collects sibling hashes and leaves needed for Merkle proofs.
//
// Multiproof-ready design:
// - requiredSiblings/requiredLeaves store which gindices we want to collect (registered before merkleization).
// - siblings/leaves store the actual collected hashes.
//
// Concurrency:
// - required* maps are read-only during merkleization.
// - siblings/leaves writes are protected by mutex.
type proofCollector struct {
	sync.Mutex

	// Required gindices (registered before merkleization)
	requiredSiblings map[uint64]struct{}
	requiredLeaves   map[uint64]struct{}

	// Collected hashes
	siblings map[uint64][32]byte
	leaves   map[uint64][32]byte
}

func newProofCollector() *proofCollector {
	return &proofCollector{
		requiredSiblings: make(map[uint64]struct{}),
		requiredLeaves:   make(map[uint64]struct{}),
		siblings:         make(map[uint64][32]byte),
		leaves:           make(map[uint64][32]byte),
	}
}

func (pc *proofCollector) reset() {
	pc.Lock()
	defer pc.Unlock()

	pc.requiredSiblings = make(map[uint64]struct{})
	pc.requiredLeaves = make(map[uint64]struct{})
	pc.siblings = make(map[uint64][32]byte)
	pc.leaves = make(map[uint64][32]byte)
}

// addTarget register the target leaf and its required sibling nodes for proof construction.
// Registration should happen before merkleization begins.
func (pc *proofCollector) addTarget(gindex uint64) {
	pc.Lock()
	defer pc.Unlock()

	pc.requiredLeaves[gindex] = struct{}{}

	// Walk from the target leaf up to (but not including) the root (gindex=1).
	// At each step, register the sibling node required to prove inclusion.
	nodeGindex := gindex
	for nodeGindex > 1 {
		siblingGindex := nodeGindex ^ 1 // flip the last bit: left<->right sibling
		pc.requiredSiblings[siblingGindex] = struct{}{}

		// Move to parent
		nodeGindex /= 2
	}
}

// toProof converts the collected siblings and leaves into a fastssz.Proof structure.
// Current behavior expects a single target leaf (single proof).
func (pc *proofCollector) toProof() (*fastssz.Proof, error) {
	pc.Lock()
	defer pc.Unlock()

	proof := &fastssz.Proof{}
	if len(pc.leaves) == 0 {
		return nil, errors.New("no leaves collected: add target leaves before merkleization")
	}

	leafGindices := make([]uint64, 0, len(pc.leaves))
	for g := range pc.leaves {
		leafGindices = append(leafGindices, g)
	}
	slices.Sort(leafGindices)

	// single proof resides in leafGindices[0]
	targetGindex := leafGindices[0]
	proofIndex, err := math.Int(targetGindex)
	if err != nil {
		return nil, fmt.Errorf("gindex %d overflows int: %w", targetGindex, err)
	}
	proof.Index = proofIndex

	// store the leaf
	leaf := pc.leaves[targetGindex]
	leafBuf := make([]byte, 32)
	copy(leafBuf, leaf[:])
	proof.Leaf = leafBuf

	// Walk from target up to root, collecting siblings.
	steps := bits.Len64(targetGindex) - 1
	proof.Hashes = make([][]byte, 0, steps)

	for targetGindex > 1 {
		sib := targetGindex ^ 1
		h, ok := pc.siblings[sib]
		if !ok {
			return nil, fmt.Errorf("missing sibling hash for gindex %d", sib)
		}
		proof.Hashes = append(proof.Hashes, h[:])
		targetGindex /= 2
	}

	return proof, nil
}

// collectLeaf checks if the given gindex is a required leaf for the proof,
// and if so, stores the provided leaf hash in the collector.
func (pc *proofCollector) collectLeaf(gindex uint64, leaf [32]byte) {
	if _, ok := pc.requiredLeaves[gindex]; !ok {
		return
	}
	pc.Lock()
	pc.leaves[gindex] = leaf
	pc.Unlock()
}

// collectSibling stores the hash for a sibling node identified by gindex.
// It only stores the hash if gindex was pre-registered via addTarget (present in requiredSiblings).
// Writes to the collected siblings map are protected by the collector mutex.
func (pc *proofCollector) collectSibling(gindex uint64, hash [32]byte) {
	if _, ok := pc.requiredSiblings[gindex]; !ok {
		return
	}
	pc.Lock()
	pc.siblings[gindex] = hash
	pc.Unlock()
}

// Merkleizers and proof collection methods

// merkleize recursively traverses an SSZ info and computes the Merkle root of the subtree.
//
// Proof collection:
//   - During traversal it calls collectLeaf/collectSibling with the SSZ generalized indices (gindices)
//     of visited nodes.
//   - The collector only stores hashes for gindices that were pre-registered via addTarget
//     (requiredLeaves/requiredSiblings). This makes the traversal multiproof-ready: you can register
//     multiple targets before calling merkleize.
//
// SSZ types handled: basic types, containers, lists, vectors, bitlists, and bitvectors.
//
// Parameters:
// - info: SSZ type metadata for the current value.
// - v: reflect.Value of the current value.
// - currentGindex: generalized index of the current subtree root.
//
// Returns:
// - [32]byte: Merkle root of the current subtree.
// - error: any error encountered during traversal/merkleization.
func (pc *proofCollector) merkleize(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	if info.sszType.isBasic() {
		return pc.merkleizeBasicType(info.sszType, v, currentGindex)
	}
	switch info.sszType {
	case Container:
		return pc.merkleizeContainer(info, v, currentGindex)
	case List:
		return pc.merkleizeList(info, v, currentGindex)
	case Vector:
		return pc.merkleizeVector(info, v, currentGindex)
	case Bitlist:
		return pc.merkleizeBitlist(info, v, currentGindex)
	case Bitvector:
		return pc.merkleizeBitvector(info, v, currentGindex)
	default:
		return [32]byte{}, fmt.Errorf("unsupported SSZ type: %v", info.sszType)
	}
}

// merkleizeBasicType serializes a basic SSZ value into a 32-byte leaf chunk (little-endian, zero-padded).
//
// Proof collection:
// - It calls collectLeaf(currentGindex, leaf) and stores the leaf if currentGindex was pre-registered via addTarget.
//
// Parameters:
// - t: the SSZType (basic).
// - v: the reflect.Value of the basic value.
// - currentGindex: the generalized index (gindex) of this leaf.
//
// Returns:
// - [32]byte: the 32-byte SSZ leaf chunk.
// - error: if the SSZType is not a supported basic type.
func (pc *proofCollector) merkleizeBasicType(t SSZType, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	var leaf [32]byte

	// Serialize the value into a 32-byte chunk (little-endian, zero-padded)
	switch t {
	case Uint8:
		leaf[0] = uint8(v.Uint())
	case Uint16:
		binary.LittleEndian.PutUint16(leaf[:2], uint16(v.Uint()))
	case Uint32:
		binary.LittleEndian.PutUint32(leaf[:4], uint32(v.Uint()))
	case Uint64:
		binary.LittleEndian.PutUint64(leaf[:8], v.Uint())
	case Boolean:
		if v.Bool() {
			leaf[0] = 1
		}
	default:
		return [32]byte{}, fmt.Errorf("unexpected basic type: %v", t)
	}

	pc.collectLeaf(currentGindex, leaf)

	return leaf, nil
}

// merkleizeContainer computes the Merkle root of an SSZ container by:
//  1. Merkleizing each field into a 32-byte subtree root
//  2. Merkleizing the field roots into the container root (padding to the next power-of-2)
//
// Generalized indices (gindices): depth = ssz.Depth(uint64(N)) and field i has gindex = (currentGindex << depth) + uint64(i).
// Proof collection: merkleize() computes each field root, merkleizeVectorAndCollect collects required siblings, and collectLeaf stores the container root if registered.
//
// Parameters:
// - info: SSZ type metadata for the container.
// - v: reflect.Value of the container value.
// - currentGindex: generalized index (gindex) of the container root.
//
// Returns:
// - [32]byte: Merkle root of the container.
// - error: any error encountered while merkleizing fields.
func (pc *proofCollector) merkleizeContainer(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	// If the container root itself is the target, compute directly and return early.
	// This avoids full subtree merkleization when we only need the root.
	if _, ok := pc.requiredLeaves[currentGindex]; ok {
		root, err := info.HashTreeRoot()
		if err != nil {
			return [32]byte{}, err
		}
		pc.collectLeaf(currentGindex, root)
		return root, nil
	}

	ci, err := info.ContainerInfo()
	if err != nil {
		return [32]byte{}, err
	}

	v = dereferencePointer(v)

	// Calculate depth: how many levels from container root to field leaves
	numFields := len(ci.order)
	depth := ssz.Depth(uint64(numFields))

	// Step 1: Compute HTR for each subtree (field)
	fieldRoots := make([][32]byte, numFields)

	for i, name := range ci.order {
		fieldInfo := ci.fields[name]
		fieldVal := v.FieldByName(fieldInfo.goFieldName)

		// Field i's gindex: shift currentGindex left by depth, then OR with field index
		fieldGindex := currentGindex<<depth + uint64(i)

		htr, err := pc.merkleize(fieldInfo.sszInfo, fieldVal, fieldGindex)
		if err != nil {
			return [32]byte{}, fmt.Errorf("field %s: %w", name, err)
		}
		fieldRoots[i] = htr
	}

	// Step 2: Merkleize the field hashes into the container root,
	// collecting sibling hashes if target is within this subtree
	root := pc.merkleizeVectorAndCollect(fieldRoots, currentGindex, uint64(depth))

	return root, nil
}

// merkleizeVectorBody computes the Merkle root of the "data" subtree for vector-like SSZ types
// (vectors and the data-part of lists/bitlists).
//
// Generalized indices (gindices): depth = ssz.Depth(limit); leafBase = subtreeRootGindex << depth; element/chunk i gindex = leafBase + uint64(i).
// Proof collection: merkleize() is called for composite elements; merkleizeVectorAndCollect collects required siblings at this layer.
// Padding: merkleizeVectorAndCollect uses trie.ZeroHashes as needed.
//
// Parameters:
// - elemInfo: SSZ type metadata for the element.
// - v: reflect.Value of the vector/list data.
// - length: number of actual elements present.
// - limit: virtual leaf capacity used for padding/Depth (fixed length for vectors, limit for lists).
// - subtreeRootGindex: gindex of the data subtree root.
//
// Returns:
// - [32]byte: Merkle root of the data subtree.
// - error: any error encountered while merkleizing composite elements.
func (pc *proofCollector) merkleizeVectorBody(elemInfo *SszInfo, v reflect.Value, length int, limit uint64, subtreeRootGindex uint64) ([32]byte, error) {
	depth := uint64(ssz.Depth(limit))

	var chunks [][32]byte
	if elemInfo.sszType.isBasic() {
		// Serialize basic elements and pack into 32-byte chunks using ssz.PackByChunk.
		elemSize, err := math.Int(itemLength(elemInfo))
		if err != nil {
			return [32]byte{}, fmt.Errorf("element size %d overflows int: %w", itemLength(elemInfo), err)
		}
		serialized := make([][]byte, length)
		// Single contiguous allocation for all element data
		allData := make([]byte, length*elemSize)
		for i := range length {
			buf := allData[i*elemSize : (i+1)*elemSize]
			elem := v.Index(i)
			if elemInfo.sszType == Boolean && elem.Bool() {
				buf[0] = 1
			} else {
				bytesutil.PutLittleEndian(buf, elem.Uint(), elemSize)
			}
			serialized[i] = buf
		}
		chunks, err = ssz.PackByChunk(serialized)
		if err != nil {
			return [32]byte{}, err
		}
	} else {
		// Composite elements: compute each element root (no padding here; merkleizeVectorAndCollect pads).
		chunks = make([][32]byte, length)

		// Parallel per-element merkleization with proper gindices for proof collection.
		var g errgroup.Group
		g.SetLimit(runtime.GOMAXPROCS(0))

		for i := range length {
			g.Go(func() error {
				elemGindex := subtreeRootGindex<<depth + uint64(i)
				htr, err := pc.merkleize(elemInfo, v.Index(i), elemGindex)
				if err != nil {
					return fmt.Errorf("index %d: %w", i, err)
				}
				chunks[i] = htr
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return [32]byte{}, fmt.Errorf("merkelize vector body concurrently: %w", err)
		}
	}

	root := pc.merkleizeVectorAndCollect(chunks, subtreeRootGindex, depth)
	return root, nil
}

// merkleizeVector computes the Merkle root of an SSZ vector (fixed-length).
//
// Generalized indices (gindices): currentGindex is the gindex of the vector root; element/chunk gindices are derived
// inside merkleizeVectorBody using leafBase = currentGindex << ssz.Depth(leaves).
//
// Proof collection: merkleizeVectorBody performs element/chunk merkleization and collects required siblings at the
// vector layer; collectLeaf stores the vector root if currentGindex was registered via addTarget.
//
// Parameters:
// - info: SSZ type metadata for the vector.
// - v: reflect.Value of the vector value.
// - currentGindex: generalized index (gindex) of the vector root.
//
// Returns:
// - [32]byte: Merkle root of the vector.
// - error: any error encountered while merkleizing composite elements.
func (pc *proofCollector) merkleizeVector(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	vi, err := info.VectorInfo()
	if err != nil {
		return [32]byte{}, err
	}

	length, err := math.Int(vi.Length())
	if err != nil {
		return [32]byte{}, fmt.Errorf("vector length %d overflows int: %w", vi.Length(), err)
	}
	elemInfo := vi.element

	// Determine the virtual leaf capacity for the vector.
	leaves, err := getChunkCount(info)
	if err != nil {
		return [32]byte{}, err
	}

	root, err := pc.merkleizeVectorBody(elemInfo, v, length, leaves, currentGindex)
	if err != nil {
		return [32]byte{}, err
	}

	// If the vector root itself is the target
	pc.collectLeaf(currentGindex, root)

	return root, nil
}

// merkleizeList computes the Merkle root of an SSZ list by merkleizing its data subtree and mixing in the length.
//
// Generalized indices (gindices): dataRoot is the left child of the list root (dataRootGindex = currentGindex*2); the length mixin is the right child (currentGindex*2+1).
// Proof collection: merkleizeVectorBody computes the data root (collecting required siblings in the data subtree), and mixinLengthAndCollect collects required siblings at the length-mixin level; collectLeaf stores the list root if registered.
//
// Parameters:
// - info: SSZ type metadata for the list.
// - v: reflect.Value of the list value.
// - currentGindex: generalized index (gindex) of the list root.
//
// Returns:
// - [32]byte: Merkle root of the list.
// - error: any error encountered while merkleizing the data subtree.
func (pc *proofCollector) merkleizeList(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	li, err := info.ListInfo()
	if err != nil {
		return [32]byte{}, err
	}

	length := v.Len()
	elemInfo := li.element

	chunks := make([][32]byte, 2)
	// Compute the length hash (little-endian uint256)
	binary.LittleEndian.PutUint64(chunks[1][:8], uint64(length))

	// Data subtree root is the left child of the list root.
	dataRootGindex := currentGindex * 2

	// Compute virtual leaf capacity for the data subtree.
	leaves, err := getChunkCount(info)
	if err != nil {
		return [32]byte{}, err
	}

	chunks[0], err = pc.merkleizeVectorBody(elemInfo, v, length, leaves, dataRootGindex)
	if err != nil {
		return [32]byte{}, err
	}

	// Handle the length mixin level (and proof bookkeeping at this level).
	// Compute the final list root: hash(dataRoot || lengthHash)
	root := pc.mixinLengthAndCollect(currentGindex, chunks)

	// If the list root itself is the target
	pc.collectLeaf(currentGindex, root)

	return root, nil
}

// merkleizeBitvectorBody computes the Merkle root of a bitvector-like byte sequence by packing it into 32-byte chunks
// and merkleizing those chunks as a fixed-capacity vector (padding with trie.ZeroHashes as needed).
//
// Generalized indices (gindices): depth = ssz.Depth(chunkLimit); leafBase = subtreeRootGindex << depth; chunk i uses gindex = leafBase + uint64(i).
// Proof collection: merkleizeVectorAndCollect collects required sibling hashes at the chunk-merkleization layer.
//
// Parameters:
// - data: raw byte sequence representing the bitvector payload.
// - chunkLimit: fixed/limit number of 32-byte chunks (used for padding/Depth).
// - subtreeRootGindex: gindex of the bitvector data subtree root.
//
// Returns:
// - [32]byte: Merkle root of the bitvector data subtree.
// - error: any error encountered while packing data into chunks.
func (pc *proofCollector) merkleizeBitvectorBody(data []byte, chunkLimit uint64, subtreeRootGindex uint64) ([32]byte, error) {
	depth := ssz.Depth(chunkLimit)
	chunks, err := ssz.PackByChunk([][]byte{data})
	if err != nil {
		return [32]byte{}, err
	}
	root := pc.merkleizeVectorAndCollect(chunks, subtreeRootGindex, uint64(depth))
	return root, nil
}

// merkleizeBitvector computes the Merkle root of a fixed-length SSZ bitvector and collects proof nodes for targets.
//
// Parameters:
// - info: SSZ type metadata for the bitvector.
// - v: reflect.Value of the bitvector value.
// - currentGindex: generalized index (gindex) of the bitvector root.
//
// Returns:
// - [32]byte: Merkle root of the bitvector.
// - error: any error encountered during packing or merkleization.
func (pc *proofCollector) merkleizeBitvector(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	bitvectorBytes := v.Bytes()
	if len(bitvectorBytes) == 0 {
		return [32]byte{}, fmt.Errorf("bitvector field is uninitialized (nil or empty slice)")
	}

	// Compute virtual leaf capacity for the bitvector.
	numChunks, err := getChunkCount(info)
	if err != nil {
		return [32]byte{}, err
	}

	root, err := pc.merkleizeBitvectorBody(bitvectorBytes, numChunks, currentGindex)
	if err != nil {
		return [32]byte{}, err
	}

	pc.collectLeaf(currentGindex, root)

	return root, nil
}

// merkleizeBitlist computes the Merkle root of an SSZ bitlist by merkleizing its data chunks and mixing in the bit length.
//
// Generalized indices (gindices): dataRoot is the left child (dataRootGindex = currentGindex*2) and the length mixin is the right child (currentGindex*2+1).
// Proof collection: merkleizeBitvectorBody computes the data root (collecting required siblings under dataRootGindex), and mixinLengthAndCollect collects required siblings at the length-mixin level; collectLeaf stores the bitlist root if registered.
//
// Parameters:
// - info: SSZ type metadata for the bitlist.
// - v: reflect.Value of the bitlist value.
// - currentGindex: generalized index (gindex) of the bitlist root.
//
// Returns:
// - [32]byte: Merkle root of the bitlist.
// - error: any error encountered while merkleizing the data subtree.
func (pc *proofCollector) merkleizeBitlist(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	bi, err := info.BitlistInfo()
	if err != nil {
		return [32]byte{}, err
	}

	bitlistBytes := v.Bytes()

	// Use go-bitfield to get bytes with termination bit cleared
	bl := bitfield.Bitlist(bitlistBytes)
	data := bl.BytesNoTrim()

	// Get the bit length from bitlistInfo
	bitLength := bi.Length()

	// Get the chunk limit from getChunkCount
	limitChunks, err := getChunkCount(info)
	if err != nil {
		return [32]byte{}, err
	}

	chunks := make([][32]byte, 2)
	// Compute the length hash (little-endian uint256)
	binary.LittleEndian.PutUint64(chunks[1][:8], uint64(bitLength))

	dataRootGindex := currentGindex * 2
	chunks[0], err = pc.merkleizeBitvectorBody(data, limitChunks, dataRootGindex)
	if err != nil {
		return [32]byte{}, err
	}

	// Handle the length mixin level (and proof bookkeeping at this level).
	root := pc.mixinLengthAndCollect(currentGindex, chunks)

	pc.collectLeaf(currentGindex, root)

	return root, nil
}

// merkleizeVectorAndCollect merkleizes a slice of 32-byte leaf nodes into a subtree root, padding to a virtual size of 2^depth.
//
// Generalized indices (gindices): at layer i (0-based), nodes have gindices levelBase = subtreeGeneralizedIndex << (depth-i) and node gindex = levelBase + idx.
// Proof collection: for each layer it calls collectSibling(nodeGindex, nodeHash) and stores only those gindices registered via addTarget.
//
// Parameters:
// - elements: leaf-level hashes (may be shorter than 2^depth; padding is applied with trie.ZeroHashes).
// - subtreeGeneralizedIndex: gindex of the subtree root.
// - depth: number of merkleization layers from subtree root to leaves.
//
// Returns:
// - [32]byte: Merkle root of the subtree.
func (pc *proofCollector) merkleizeVectorAndCollect(elements [][32]byte, subtreeGeneralizedIndex uint64, depth uint64) [32]byte {
	// Return zerohash at depth
	if len(elements) == 0 {
		return trie.ZeroHashes[depth]
	}
	for i := range depth {
		layerLen := len(elements)
		oddNodeLength := layerLen%2 == 1
		if oddNodeLength {
			zerohash := trie.ZeroHashes[i]
			elements = append(elements, zerohash)
		}

		levelBaseGindex := subtreeGeneralizedIndex << (depth - i)
		for idx := range elements {
			gindex := levelBaseGindex + uint64(idx)
			pc.collectSibling(gindex, elements[idx])
			pc.collectLeaf(gindex, elements[idx])
		}

		elements = htr.VectorizedSha256(elements)
	}
	return elements[0]
}

// mixinLengthAndCollect computes the final mix-in root for list/bitlist values:
//
//	root = hash(dataRoot, lengthHash)
//
// where chunks[0] is dataRoot and chunks[1] is the 32-byte length hash.
//
// Generalized indices (gindices): dataRoot is the left child (dataRootGindex = currentGindex*2) and lengthHash is the right child (lengthHashGindex = currentGindex*2+1).
// Proof collection: it calls collectSibling/collectLeaf for both child gindices; the collector stores them only if they were registered via addTarget.
//
// Parameters:
// - currentGindex: gindex of the parent node (list/bitlist root).
// - chunks: two 32-byte nodes: [dataRoot, lengthHash].
//
// Returns:
// - [32]byte: mixed-in Merkle root (or zero value on hashing error).
// - error: any error encountered during hashing.
func (pc *proofCollector) mixinLengthAndCollect(currentGindex uint64, chunks [][32]byte) [32]byte {
	dataRoot, lengthHash := chunks[0], chunks[1]
	dataRootGindex, lengthHashGindex := currentGindex*2, currentGindex*2+1

	pc.collectSibling(dataRootGindex, dataRoot)
	pc.collectSibling(lengthHashGindex, lengthHash)

	pc.collectLeaf(dataRootGindex, dataRoot)
	pc.collectLeaf(lengthHashGindex, lengthHash)

	return ssz.MixInLength(dataRoot, lengthHash[:])
}
