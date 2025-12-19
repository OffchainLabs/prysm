package query

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"reflect"
	"runtime"
	"slices"
	"sync"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	ssz "github.com/OffchainLabs/prysm/v7/encoding/ssz"
	fastssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/gohashtree"
)

// ProofCollector collects sibling hashes and leaves needed for Merkle proofs.
//
// Multiproof-ready design:
// - requiredSiblings/requiredLeaves store which gindices we want to collect (registered before merkleization).
// - siblings/leaves store the actual collected hashes.
//
// Concurrency:
// - required* maps are read-only during merkleization.
// - siblings/leaves writes are protected by mu.
type ProofCollector struct {
	mu sync.Mutex

	// Required gindices (registered before merkleization)
	requiredSiblings map[uint64]struct{}
	requiredLeaves   map[uint64]struct{}

	// Collected hashes
	siblings map[uint64][32]byte
	leaves   map[uint64][32]byte
}

func NewProofCollector() *ProofCollector {
	return &ProofCollector{
		requiredSiblings: make(map[uint64]struct{}),
		requiredLeaves:   make(map[uint64]struct{}),
		siblings:         make(map[uint64][32]byte),
		leaves:           make(map[uint64][32]byte),
	}
}

func (pc *ProofCollector) Reset() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.requiredSiblings = make(map[uint64]struct{})
	pc.requiredLeaves = make(map[uint64]struct{})
	pc.siblings = make(map[uint64][32]byte)
	pc.leaves = make(map[uint64][32]byte)
}

// AddTarget register the target leaf and its required sibling nodes for proof construction.
// Registration should happen before merkleization begins.
func (pc *ProofCollector) AddTarget(gindex uint64) {
	// Lock safe just in case the collector is re-used.
	pc.mu.Lock()
	defer pc.mu.Unlock()

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
func (pc *ProofCollector) toProof() *fastssz.Proof {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	proof := &fastssz.Proof{}
	if len(pc.leaves) == 0 {
		return proof
	}

	leafGindices := make([]uint64, 0, len(pc.leaves))
	for g := range pc.leaves {
		leafGindices = append(leafGindices, g)
	}
	slices.Sort(leafGindices)

	// single proof resides in leafGindices[0]
	targetGindex := leafGindices[0]
	proof.Index = int(targetGindex)

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
			// Missing sibling; proof will fail verification.
			break
		}
		proof.Hashes = append(proof.Hashes, h[:])
		targetGindex /= 2
	}

	return proof
}

// registerRequiredSiblings computes all sibling generalized indices along the path
// from the given gindex up to the root. These are the nodes whose hashes
// are needed to construct a merkle proof.
func (pc *ProofCollector) registerRequiredSiblings(gindex uint64) {
	pc.Reset()
	pc.AddTarget(gindex)
}

// collectLeaf checks if the given gindex is a required leaf for the proof,
// and if so, stores the provided leaf hash in the collector.
func (pc *ProofCollector) collectLeaf(gindex uint64, leaf [32]byte) {
	if _, ok := pc.requiredLeaves[gindex]; !ok {
		return
	}
	pc.mu.Lock()
	pc.leaves[gindex] = leaf
	pc.mu.Unlock()
}

// Merkleizers and proof collection methods

// merkleize recursively traverses an SSZ info  and computes the Merkle root of the subtree.
//
// Proof collection:
//   - During traversal it calls collectLeaf/collectSibling with the SSZ generalized indices (gindices)
//     of visited nodes.
//   - The collector only stores hashes for gindices that were pre-registered via AddTarget
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
func (pc *ProofCollector) merkleize(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
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
// - It calls collectLeaf(currentGindex, leaf) and stores the leaf if currentGindex was pre-registered via AddTarget.
//
// Parameters:
// - t: the SSZType (basic).
// - v: the reflect.Value of the basic value.
// - currentGindex: the generalized index (gindex) of this leaf.
//
// Returns:
// - [32]byte: the 32-byte SSZ leaf chunk.
// - error: if the SSZType is not a supported basic type.
func (pc *ProofCollector) merkleizeBasicType(t SSZType, v reflect.Value, currentGindex uint64) ([32]byte, error) {
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
// Proof collection: merkleize() computes each field root, MerkleizeVectorAndCollect collects required siblings, and collectLeaf stores the container root if registered.
//
// Parameters:
// - info: SSZ type metadata for the container.
// - v: reflect.Value of the container value.
// - currentGindex: generalized index (gindex) of the container root.
//
// Returns:
// - [32]byte: Merkle root of the container.
// - error: any error encountered while merkleizing fields.
func (pc *ProofCollector) merkleizeContainer(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
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
	root := pc.MerkleizeVectorAndCollect(fieldRoots, currentGindex, uint64(depth))

	// If the container root itself is the target
	pc.collectLeaf(currentGindex, root)

	return root, nil
}

// merkleizeVectorBody computes the Merkle root of the "data" subtree for vector-like SSZ types
// (vectors and the data-part of lists/bitlists).
//
// Generalized indices (gindices): depth = ssz.Depth(limit); leafBase = subtreeRootGindex << depth; element/chunk i gindex = leafBase + uint64(i).
// Proof collection: merkleize() is called for composite elements; MerkleizeVectorAndCollect collects required siblings at this layer.
// Padding: MerkleizeVectorAndCollect uses trie.ZeroHashes as needed.
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
func (pc *ProofCollector) merkleizeVectorBody(elemInfo *SszInfo, v reflect.Value, length int, limit uint64, subtreeRootGindex uint64) ([32]byte, error) {
	depth := int(ssz.Depth(limit))

	var chunks [][32]byte
	if elemInfo.sszType.isBasic() {
		// Pack basic elements into 32-byte chunks.
		chunks = packBasicElementsToChunks(elemInfo, v, length)
	} else {
		// Composite elements: compute each element root (no padding here; MerkleizeVectorAndCollect pads).
		chunks = make([][32]byte, length)

		// Parallel execution
		workerCount := runtime.GOMAXPROCS(0) * 2
		if workerCount > length {
			workerCount = length
		}

		jobs := make(chan int, workerCount*16)
		errCh := make(chan error, 1) // only need the first error
		stopCh := make(chan struct{})
		var stopOnce sync.Once
		var wg sync.WaitGroup

		worker := func() {
			defer wg.Done()
			for idx := range jobs {
				select {
				case <-stopCh:
					return
				default:
				}

				elemGindex := subtreeRootGindex<<depth + uint64(idx)
				htr, err := pc.merkleize(elemInfo, v.Index(idx), elemGindex)
				if err != nil {
					stopOnce.Do(func() { close(stopCh) })
					select {
					case errCh <- fmt.Errorf("index %d: %w", idx, err):
					default:
					}
					return
				}
				chunks[idx] = htr
			}
		}

		wg.Add(workerCount)
		for w := 0; w < workerCount; w++ {
			go worker()
		}

		// Enqueue jobs; stop early if any worker reports an error.
	enqueue:
		for i := 0; i < length; i++ {
			select {
			case <-stopCh:
				break enqueue
			case jobs <- i:
			}
		}
		close(jobs)

		wg.Wait()

		select {
		case err := <-errCh:
			return [32]byte{}, err
		default:
		}
	}

	root := pc.MerkleizeVectorAndCollect(chunks, subtreeRootGindex, uint64(depth))
	return root, nil
}

// merkleizeVector computes the Merkle root of an SSZ vector (fixed-length).
//
// Generalized indices (gindices): currentGindex is the gindex of the vector root; element/chunk gindices are derived
// inside merkleizeVectorBody using leafBase = currentGindex << ssz.Depth(leaves).
//
// Proof collection: merkleizeVectorBody performs element/chunk merkleization and collects required siblings at the
// vector layer; collectLeaf stores the vector root if currentGindex was registered via AddTarget.
//
// Parameters:
// - info: SSZ type metadata for the vector.
// - v: reflect.Value of the vector value.
// - currentGindex: generalized index (gindex) of the vector root.
//
// Returns:
// - [32]byte: Merkle root of the vector.
// - error: any error encountered while merkleizing composite elements.
func (pc *ProofCollector) merkleizeVector(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	vi, err := info.VectorInfo()
	if err != nil {
		return [32]byte{}, err
	}

	length := v.Len()
	elemInfo := vi.element

	// Determine the virtual leaf capacity for the vector.
	// For composite vectors: leaves == fixed element count.
	// For packed-basic vectors: leaves == fixed chunk count.
	var leaves uint64
	if elemInfo.sszType.isBasic() {
		elemLen := itemLength(elemInfo)
		leaves = (uint64(length)*elemLen + 31) / 32
	} else {
		leaves = uint64(length)
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
func (pc *ProofCollector) merkleizeList(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	li, err := info.ListInfo()
	if err != nil {
		return [32]byte{}, err
	}

	length := v.Len()
	limit := li.Limit()
	elemInfo := li.element

	chunks := make([][32]byte, 2)
	// Compute the length hash (little-endian uint256)
	binary.LittleEndian.PutUint64(chunks[1][:8], uint64(length))

	// Data subtree root is the left child of the list root.
	dataRootGindex := currentGindex * 2

	// Compute virtual leaf capacity for the data subtree.
	// Note: List[T, 0] is illegal per SSZ spec, so limit > 0 is guaranteed.
	var leaves uint64
	if elemInfo.sszType.isBasic() {
		// Packed-basic list: leaves is the limit in 32-byte chunks.
		leaves = fastssz.CalculateLimit(limit, uint64(length), itemLength(elemInfo))
	} else {
		// Composite list: leaves is the element limit.
		leaves = uint64(limit)
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
// Proof collection: MerkleizeVectorAndCollect collects required sibling hashes at the chunk-merkleization layer.
//
// Parameters:
// - data: raw byte sequence representing the bitvector payload.
// - chunkLimit: fixed/limit number of 32-byte chunks (used for padding/Depth).
// - subtreeRootGindex: gindex of the bitvector data subtree root.
//
// Returns:
// - [32]byte: Merkle root of the bitvector data subtree.
// - error: any error encountered while packing data into chunks.
func (pc *ProofCollector) merkleizeBitvectorBody(data []byte, chunkLimit uint64, subtreeRootGindex uint64) ([32]byte, error) {
	depth := ssz.Depth(chunkLimit)
	chunks, err := ssz.PackByChunk([][]byte{data})
	if err != nil {
		return [32]byte{}, err
	}
	root := pc.MerkleizeVectorAndCollect(chunks, subtreeRootGindex, uint64(depth))
	return root, nil
}

func (pc *ProofCollector) merkleizeBitvector(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	bv, err := info.BitvectorInfo()
	if err != nil {
		return [32]byte{}, err
	}

	bitvectorBytes := v.Bytes()
	if len(bitvectorBytes) == 0 {
		return [32]byte{}, fmt.Errorf("bitvector field is uninitialized (nil or empty slice)")
	}

	// Fixed bitvector length -> fixed number of 32-byte chunks.
	// Note: Bitvector[0] is illegal per SSZ spec, so Length() >= 1 is guaranteed.
	numChunks := (bv.Length() + 255) / 256

	root, err := pc.merkleizeBitvectorBody(bitvectorBytes, uint64(numChunks), currentGindex)
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
func (pc *ProofCollector) merkleizeBitlist(info *SszInfo, v reflect.Value, currentGindex uint64) ([32]byte, error) {
	bi, err := info.BitlistInfo()
	if err != nil {
		return [32]byte{}, err
	}

	bitlistBytes := v.Bytes()
	// Handle zero-initialized bitlist: create a single byte with just the termination bit
	if len(bitlistBytes) == 0 {
		bitlistBytes = []byte{0x01}
	}

	// Use go-bitfield to get length and bytes with termination bit cleared
	bl := bitfield.Bitlist(bitlistBytes)
	data := bl.BytesNoTrim()
	bitLength := bl.Len() // number of bits (excluding termination bit)

	// limit is in bits; convert to fixed number of 256-bit chunks.
	// Note: Bitlist[0] is illegal per SSZ spec, so limit >= 1 is guaranteed.
	limitChunks := (bi.limit + 255) / 256

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

// MerkleizeVectorAndCollect merkleizes a slice of 32-byte leaf nodes into a subtree root, padding to a virtual size of 2^depth.
//
// Generalized indices (gindices): at layer i (0-based), nodes have gindices levelBase = subtreeGeneralizedIndex << (depth-i) and node gindex = levelBase + idx.
// Proof collection: for each layer it calls collectSibling(nodeGindex, nodeHash) and stores only those gindices registered via AddTarget.
//
// Parameters:
// - elements: leaf-level hashes (may be shorter than 2^depth; padding is applied with trie.ZeroHashes).
// - subtreeGeneralizedIndex: gindex of the subtree root.
// - depth: number of merkleization layers from subtree root to leaves.
//
// Returns:
// - [32]byte: Merkle root of the subtree.
func (pc *ProofCollector) MerkleizeVectorAndCollect(elements [][32]byte, subtreeGeneralizedIndex uint64, depth uint64) [32]byte {
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
// Proof collection: it calls collectSibling/collectLeaf for both child gindices; the collector stores them only if they were registered via AddTarget.
//
// Parameters:
// - currentGindex: gindex of the parent node (list/bitlist root).
// - chunks: two 32-byte nodes: [dataRoot, lengthHash].
//
// Returns:
// - [32]byte: mixed-in Merkle root (or zero value on hashing error).
func (pc *ProofCollector) mixinLengthAndCollect(currentGindex uint64, chunks [][32]byte) [32]byte {
	dataRootGindex := currentGindex * 2
	lengthHashGindex := currentGindex*2 + 1

	// Check if dataRoot is a sibling we need to collect
	pc.collectSibling(dataRootGindex, chunks[0])

	// Check if lengthHash is a sibling we need to collect
	pc.collectSibling(lengthHashGindex, chunks[1])

	// Check if dataRoot is a leaf we need to collect
	pc.collectLeaf(dataRootGindex, chunks[0])

	// Check if lengthHash is a leaf we need to collect
	pc.collectLeaf(lengthHashGindex, chunks[1])

	if err := gohashtree.Hash(chunks, chunks); err != nil {
		return [32]byte{}
	}
	return chunks[0]
}

// collectSibling stores the hash for a sibling node identified by gindex.
// It only stores the hash if gindex was pre-registered via AddTarget (present in requiredSiblings).
// Writes to the collected siblings map are protected by the collector mutex.
func (pc *ProofCollector) collectSibling(gindex uint64, hash [32]byte) {
	if _, ok := pc.requiredSiblings[gindex]; !ok {
		return
	}
	pc.mu.Lock()
	pc.siblings[gindex] = hash
	pc.mu.Unlock()
}

// Utils

// packBasicElementsToChunks packs basic type elements into 32-byte chunks.
// Returns slice of chunks as [32]byte arrays.
func packBasicElementsToChunks(elemInfo *SszInfo, v reflect.Value, length int) [][32]byte {
	if length == 0 {
		return [][32]byte{{}}
	}

	elemSize := int(itemLength(elemInfo))
	elemsPerChunk := 32 / elemSize
	numChunks := (length + elemsPerChunk - 1) / elemsPerChunk

	chunks := make([][32]byte, numChunks)
	for chunkIdx := 0; chunkIdx < numChunks; chunkIdx++ {
		for i := 0; i < elemsPerChunk; i++ {
			elemIdx := chunkIdx*elemsPerChunk + i
			if elemIdx >= length {
				break
			}
			offset := i * elemSize
			if elemInfo.sszType == Boolean {
				if v.Index(elemIdx).Bool() {
					chunks[chunkIdx][offset] = 1
				}
			} else {
				putLittleEndian(chunks[chunkIdx][offset:], v.Index(elemIdx).Uint(), elemSize)
			}
		}
	}

	return chunks
}

// putLittleEndian writes an unsigned integer value in little-endian format.
// Supports sizes 1, 2, 4, or 8 bytes for uint8/16/32/64 respectively.
func putLittleEndian(dst []byte, val uint64, size int) {
	for i := 0; i < size; i++ {
		dst[i] = byte(val >> (8 * i))
	}
}
