package query

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"runtime"
	"sync"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	ssz "github.com/OffchainLabs/prysm/v7/encoding/ssz"
	fastssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/gohashtree"
)

// ProofCollector collects sibling hashes and leaves needed for a Merkle proof.
// It maps generalized indices to their corresponding hashes or leaves.
type ProofCollector struct {
	siblings map[uint64][32]byte
	leaves   map[uint64][32]byte
}

func NewProofCollector() *ProofCollector {
	return &ProofCollector{
		siblings: make(map[uint64][32]byte),
		leaves:   make(map[uint64][32]byte),
	}
}

func (pc *ProofCollector) Reset() {
	pc.siblings = make(map[uint64][32]byte)
	pc.leaves = make(map[uint64][32]byte)
}

func (pc *ProofCollector) AddTarget(gindex uint64) {
	pc.leaves[gindex] = [32]byte{} // marker

	for g := gindex; g > 1; g /= 2 {
		pc.siblings[g^1] = [32]byte{} // marker
	}
}

// toProof converts the collected siblings and leaves into a fastssz.Proof structure.
// It assumes there is only one leaf in the leaves map (the target of the proof).
func (pc *ProofCollector) toProof() *fastssz.Proof {
	proof := &fastssz.Proof{}

	// Get target gindex and leaf from leaves map (single entry for single proof)
	var targetGindex uint64
	for gindex, leaf := range pc.leaves {
		targetGindex = gindex
		proof.Index = int(gindex)
		proof.Leaf = leaf[:]
	}

	// Collect siblings in leaf-to-root order
	// Walk from target gindex up to root, collecting sibling hashes
	gindex := targetGindex
	for gindex > 1 {
		siblingGindex := gindex ^ 1
		if hash, ok := pc.siblings[siblingGindex]; ok {
			proof.Hashes = append(proof.Hashes, hash[:])
		}
		gindex = gindex / 2
	}

	return proof
}

// RegisterRequiredSiblings computes all sibling generalized indices along the path
// from the given gindex up to the root. These are the nodes whose hashes
// are needed to construct a merkle proof.
func (pc *ProofCollector) RegisterRequiredSiblings(gindex uint64) {
	pc.Reset()
	pc.AddTarget(gindex)
}

func (pc *ProofCollector) collectLeaf(gindex uint64, leaf [32]byte) {
	if _, ok := pc.leaves[gindex]; ok {
		pc.leaves[gindex] = leaf
	}
}

// Merkleizers

// merkleize recursively traverse the SSZ structure to build the Merkle proof.
// It handles basic types, containers, lists, vectors, bitlists, and bitvectors.
// Parameters:
// - info: the SszInfo for the current SSZ object.
// - v: the reflect.Value of the current SSZ object.
// - currentGindex: the generalized index of the current node in the traversal.
// Returns:
// - [32]byte: the Merkle root of the current subtree.
// - error: any error encountered during merkleization.
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

// merkleizeBasicType serializes a basic SSZ type into a 32-byte leaf chunk.
// If this leaf is the proof target (gindex == currentGindex), it sets proof.Leaf and proof.Index.
// Parameters:
// - t: the SSZType (basic).
// - v: the reflect.Value of the basic type.
// - currentGindex: the generalized index of the current node in the traversal.
// Returns:
// - [32]byte: the 32-byte leaf chunk.
// - error: if the provided data type is unexpected.
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

	// If this leaf is the target we're proving, update the proof
	pc.collectLeaf(currentGindex, leaf)

	return leaf, nil
}

// Tree structure for a container with N fields:
//
//	    container root (currentGindex)
//	       /        \
//	    ...          ...
//	   /    \      /    \
//	field0  field1 ... fieldN-1  [virtual zero subtrees for padding]
//
// Field i has gindex: (currentGindex << depth) | i, where depth = log2(nextPow2(N))
// Padding to power-of-2 is handled by MerkleizeVectorAndCollect using trie.ZeroHashes.
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
		fieldGindex := currentGindex*(1<<depth) + uint64(i)

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

// merkleizeVectorBody merkleizes the "data" part of a vector-like structure.
// - `length` is the number of actual elements present.
// - `virtualLeaves` defines the virtual leaf capacity (used for padding/Depth):
//   - vectors: virtualLeaves == fixed element count (or fixed chunk count for packed basic)
//   - lists:   virtualLeaves == limit element count (or limit chunk count for packed basic)
//
// - `subtreeRootGindex` is the gindex of the data subtree root.
func (pc *ProofCollector) merkleizeVectorBody(elemInfo *SszInfo, v reflect.Value, length int, virtualLeaves uint64, subtreeRootGindex uint64) ([32]byte, error) {
	depth := int(ssz.Depth(virtualLeaves))

	var chunks [][32]byte
	if elemInfo.sszType.isBasic() {
		// Pack basic elements into 32-byte chunks.
		chunks = packBasicElementsToChunks(elemInfo, v, length)
	} else {
		// Composite elements: compute each element root (no padding here; MerkleizeVectorAndCollect pads).
		chunks = make([][32]byte, length)

		// Parallel execution for large collections (bounded worker pool)
		workerCount := runtime.GOMAXPROCS(0) * 2
		if workerCount < 1 {
			workerCount = 1
		}
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

				elemGindex := subtreeRootGindex*(1<<depth) + uint64(idx)
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

// merkleizeVector handles SSZ vectors (fixed-length).
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

// merkleizeList handles SSZ lists (variable-length).
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

// merkleizeBitvectorBody merkleizes a chunked byte sequence as a bitvector-like structure.
// `virtualChunks` is the fixed/limit chunk capacity used for padding/Depth.
func (pc *ProofCollector) merkleizeBitvectorBody(data []byte, virtualChunks uint64, subtreeRootGindex uint64) ([32]byte, error) {
	depth := ssz.Depth(virtualChunks)
	chunks := chunkBytes(data)
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

// MerkleizeVectorAndCollect uses the optimized VectorizedSha256 routine to hash a list of 32-byte
// elements while collecting sibling hashes for proof generation.
// It is similar to MerkleizeVector but also updates the ProofCollector.
// Parameters:
// - elements: the leaf-level hashes
// - subtreeGeneralizedIndex: the gindex of this subtree's root
// - depth: the depth from subtree root to leaves (determines virtual tree size = 2^depth)
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

		// Debug: print generalized indices for the nodes at this layer.
		// At loop iteration i, `elements` represents nodes at tree depth (depth - i),
		// so the gindex base is 1<<(depth-i) and each node is base+idx.
		for idx, element := range elements {
			levelBaseGindex := subtreeGeneralizedIndex << (depth - i)
			gindex := levelBaseGindex + uint64(idx)
			pc.collectSibling(gindex, element)
		}

		elements = htr.VectorizedSha256(elements)
	}
	return elements[0]
}

// mixinLengthAndCollect handles the final mix-in layer for list/bitlist:
// root = sha256Two(dataRoot, lengthHash)
// It also updates the collector for siblings/leaves at the length mixin level.
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

func (pc *ProofCollector) collectSibling(gindex uint64, hash [32]byte) {
	if _, ok := pc.siblings[gindex]; ok {
		pc.siblings[gindex] = hash
	}
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

// chunkBytes splits a byte slice into 32-byte chunks.
// The last chunk is zero-padded if necessary.
func chunkBytes(data []byte) [][32]byte {
	if len(data) == 0 {
		return [][32]byte{{}}
	}

	numChunks := (len(data) + 31) / 32
	chunks := make([][32]byte, numChunks)

	for i := 0; i < numChunks; i++ {
		start := i * 32
		end := start + 32
		if end > len(data) {
			end = len(data)
		}
		copy(chunks[i][:], data[start:end])
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
