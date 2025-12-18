package query

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"reflect"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	ssz "github.com/OffchainLabs/prysm/v7/encoding/ssz"
	fastssz "github.com/prysmaticlabs/fastssz"
)

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

func (pc *ProofCollector) toProof() *fastssz.Proof {
	proof := &fastssz.Proof{}

	// Get target gindex and leaf from leaves map (single entry for single proof)
	var targetGindex uint64
	for gindex, leaf := range pc.leaves {
		targetGindex = gindex
		proof.Index = int(gindex)
		proof.Leaf = leaf[:]
		break
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

// siblingsGindex computes all sibling generalized indices along the path
// from the given gindex up to the root. These are the nodes whose hashes
// are needed to construct a merkle proof.
func (pc *ProofCollector) siblingsGindex(gindex uint64) {
	// Store the target gindex in leaves map
	pc.leaves[gindex] = [32]byte{}

	// Walk from gindex up to root (gindex 1)
	// At each level, the sibling is gindex XOR 1
	for gindex > 1 {
		siblingGindex := gindex ^ 1
		pc.siblings[siblingGindex] = [32]byte{}
		gindex = gindex / 2 // Move to parent
	}
}

// Prove is the entrypoint to generate an SSZ Merkle proof for the given generalized index.
// Parameters:
// - gindex: the generalized index of the node to prove inclusion for.
// Returns:
// - fastssz.Proof: the Merkle proof containing the leaf, index, and sibling hashes.
// - [32]byte: the Merkle root of the entire SSZ object.
// - error: any error encountered during proof generation.
func (info *SszInfo) Prove(gindex uint64) (*fastssz.Proof, error) {
	if info == nil {
		return nil, fmt.Errorf("nil SszInfo")
	}

	collector := NewProofCollector()
	collector.siblingsGindex(gindex)

	// info.source is guaranteed to be valid and dereferenced by AnalyzeObject
	v := reflect.ValueOf(info.source).Elem()

	_, err := merkleize(info, v, collector, 1)
	if err != nil {
		return nil, err
	}

	return collector.toProof() /*htr,*/, nil
}

// merkleize recursively traverse the SSZ structure to build the Merkle proof.
// It handles basic types, containers, lists, vectors, bitlists, and bitvectors.
// Parameters:
// - info: the SszInfo for the current SSZ object.
// - v: the reflect.Value of the current SSZ object.
// - targetGindex: the generalized index of the node to prove inclusion for.
// - currentGindex: the generalized index of the current node in the traversal.
// - proof: the fastssz.Proof being constructed.
// Returns:
// - [32]byte: the Merkle root of the current subtree.
// - error: any error encountered during merkleization.
func merkleize(info *SszInfo, v reflect.Value, collector *ProofCollector, currentGindex uint64) ([32]byte, error) {
	if info.sszType.isBasic() {
		return merkleizeBasicType(info.sszType, v, collector, currentGindex)
	}
	switch info.sszType {
	case Container:
		return merkleizeContainer(info, v, collector, currentGindex)
	case List:
		return merkleizeList(info, v, collector, currentGindex)
	case Vector:
		return merkleizeVector(info, v, collector, currentGindex)
	case Bitlist:
		return merkleizeBitlist(info, v, collector, currentGindex)
	case Bitvector:
		return merkleizeBitvector(info, v, collector, currentGindex)
	default:
		return [32]byte{}, fmt.Errorf("unsupported SSZ type: %v", info.sszType)
	}
}

// merkleizeBasicType serializes a basic SSZ type into a 32-byte leaf chunk.
// If this leaf is the proof target (gindex == currentGindex), it sets proof.Leaf and proof.Index.
// Parameters:
// - t: the SSZType (basic).
// - v: the reflect.Value of the basic type.
// - targetGindex: the generalized index of the node to prove inclusion for.
// - currentGindex: the generalized index of the current node in the traversal.
// - proof: the fastssz.Proof being constructed.
// Returns:
// - [32]byte: the 32-byte leaf chunk.
// - error: if the provided data type is unexpected.
func merkleizeBasicType(t SSZType, v reflect.Value, collector *ProofCollector, currentGindex uint64) ([32]byte, error) {
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
	if _, ok := collector.leaves[currentGindex]; ok {
		collector.leaves[currentGindex] = leaf
	}

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
// Padding to power-of-2 is handled by merkleizeWithProofCollection using trie.ZeroHashes.
func merkleizeContainer(info *SszInfo, v reflect.Value, collector *ProofCollector, currentGindex uint64) ([32]byte, error) {
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

		htr, err := merkleize(fieldInfo.sszInfo, fieldVal, collector, fieldGindex)
		if err != nil {
			return [32]byte{}, fmt.Errorf("field %s: %w", name, err)
		}
		fieldRoots[i] = htr
	}

	// Step 2: Merkleize the field hashes into the container root,
	// collecting sibling hashes if target is within this subtree
	root := merkleizeWithProofCollection(fieldRoots, collector, currentGindex, int(depth))

	// If the container root itself is the target
	if _, ok := collector.leaves[currentGindex]; ok {
		collector.leaves[currentGindex] = root
	}

	return root, nil
}

// merkleizeVectorBody merkleizes the "data" part of a vector-like structure.
// - `length` is the number of actual elements present.
// - `virtualLeaves` defines the virtual leaf capacity (used for padding/Depth):
//   - vectors: virtualLeaves == fixed element count (or fixed chunk count for packed basic)
//   - lists:   virtualLeaves == limit element count (or limit chunk count for packed basic)
//
// - `subtreeRootGindex` is the gindex of the data subtree root.
func merkleizeVectorBody(elemInfo *SszInfo, v reflect.Value, length int, virtualLeaves uint64, collector *ProofCollector, subtreeRootGindex uint64) ([32]byte, error) {
	depth := int(ssz.Depth(virtualLeaves))

	var chunks [][32]byte
	if elemInfo.sszType.isBasic() {
		// Pack basic elements into 32-byte chunks.
		chunks = packBasicElementsToChunks(elemInfo, v, length)
	} else {
		// Composite elements: compute each element root (no padding here; merkleizeWithProofCollection pads).
		chunks = make([][32]byte, length)
		for i := 0; i < length; i++ {
			elemGindex := subtreeRootGindex*(1<<depth) + uint64(i)
			htr, err := merkleize(elemInfo, v.Index(i), collector, elemGindex)
			if err != nil {
				return [32]byte{}, fmt.Errorf("index %d: %w", i, err)
			}
			chunks[i] = htr
		}
	}

	root := merkleizeWithProofCollection(chunks, collector, subtreeRootGindex, depth)
	return root, nil
}

// merkleizeVector handles SSZ vectors (fixed-length).
func merkleizeVector(info *SszInfo, v reflect.Value, collector *ProofCollector, currentGindex uint64) ([32]byte, error) {
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

	root, err := merkleizeVectorBody(elemInfo, v, length, leaves, collector, currentGindex)
	if err != nil {
		return [32]byte{}, err
	}

	// If the vector root itself is the target
	if _, ok := collector.leaves[currentGindex]; ok {
		collector.leaves[currentGindex] = root
	}

	return root, nil
}

// merkleizeList handles SSZ lists (variable-length).
func merkleizeList(info *SszInfo, v reflect.Value, collector *ProofCollector, currentGindex uint64) ([32]byte, error) {
	li, err := info.ListInfo()
	if err != nil {
		return [32]byte{}, err
	}

	length := v.Len()
	limit := li.Limit()
	elemInfo := li.element

	// Compute the length hash (little-endian uint256)
	var lengthHash [32]byte
	binary.LittleEndian.PutUint64(lengthHash[:8], uint64(length))

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

	dataRoot, err := merkleizeVectorBody(elemInfo, v, length, leaves, collector, dataRootGindex)
	if err != nil {
		return [32]byte{}, err
	}

	// Handle the length mixin level (and proof bookkeeping at this level).
	applyLengthMixin(currentGindex, dataRoot, lengthHash, collector)

	// Compute the final list root: hash(dataRoot || lengthHash)
	root := sha256Two(dataRoot, lengthHash)

	// If the list root itself is the target
	if _, ok := collector.leaves[currentGindex]; ok {
		collector.leaves[currentGindex] = root
	}

	return root, nil
}

// merkleizeBitvectorBody merkleizes a chunked byte sequence as a bitvector-like structure.
// `virtualChunks` is the fixed/limit chunk capacity used for padding/Depth.
func merkleizeBitvectorBody(data []byte, virtualChunks uint64, collector *ProofCollector, subtreeRootGindex uint64) ([32]byte, error) {
	depth := int(ssz.Depth(virtualChunks))
	chunks := chunkBytes(data)
	root := merkleizeWithProofCollection(chunks, collector, subtreeRootGindex, depth)
	return root, nil
}

func merkleizeBitvector(info *SszInfo, v reflect.Value, collector *ProofCollector, currentGindex uint64) ([32]byte, error) {
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

	root, err := merkleizeBitvectorBody(bitvectorBytes, uint64(numChunks), collector, currentGindex)
	if err != nil {
		return [32]byte{}, err
	}

	if _, ok := collector.leaves[currentGindex]; ok {
		collector.leaves[currentGindex] = root
	}

	return root, nil
}

func merkleizeBitlist(info *SszInfo, v reflect.Value, collector *ProofCollector, currentGindex uint64) ([32]byte, error) {
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

	// Compute the length hash (little-endian uint256)
	var lengthHash [32]byte
	binary.LittleEndian.PutUint64(lengthHash[:8], uint64(bitLength))

	dataRootGindex := currentGindex * 2
	dataRoot, err := merkleizeBitvectorBody(data, limitChunks, collector, dataRootGindex)
	if err != nil {
		return [32]byte{}, err
	}

	// Handle the length mixin level (and proof bookkeeping at this level).
	applyLengthMixin(currentGindex, dataRoot, lengthHash, collector)

	root := sha256Two(dataRoot, lengthHash)
	if _, ok := collector.leaves[currentGindex]; ok {
		collector.leaves[currentGindex] = root
	}

	return root, nil
}

// merkleizeWithProofCollection builds a merkle tree from chunks and collects proof hashes
// along the path from the target leaf up to the subtree root.
// Uses trie.ZeroHashes to avoid recomputing hashes for zero subtrees.
//
// Parameters:
// - chunks: the leaf-level hashes (NOT padded to power of 2 - we handle padding with ZeroHashes)
// - collector: the ProofCollector containing siblings map to populate with hashes
// - subtreeRootGindex: the gindex of this subtree's root
// - depth: the depth from subtree root to leaves (determines virtual tree size = 2^depth)
func merkleizeWithProofCollection(chunks [][32]byte, collector *ProofCollector, subtreeRootGindex uint64, depth int) [32]byte {
	if len(chunks) == 0 {
		return trie.ZeroHashes[depth]
	}

	// Build tree layer by layer, from leaves up to root
	// Like MerkleizeVector: at each layer, if odd length, append ZeroHashes[layer]
	current := chunks

	for layer := 0; layer < depth; layer++ {
		// Gindex of first node at this level (layer 0 = leaves, layer depth-1 = children of root)
		currentDepth := depth - layer
		levelBaseGindex := subtreeRootGindex << currentDepth

		layerLen := len(current)
		oddNodeLength := layerLen%2 == 1

		// If odd number of nodes, append the precomputed zero hash for this layer
		if oddNodeLength {
			zeroHash := trie.ZeroHashes[layer]
			current = append(current, zeroHash)

			// Check if the zero subtree gindex is a sibling we need to collect
			zeroGindex := levelBaseGindex + uint64(layerLen)
			if _, ok := collector.siblings[zeroGindex]; ok {
				collector.siblings[zeroGindex] = zeroHash
			}

			// Check if the last real node is a sibling we need to collect
			lastRealIndex := layerLen - 1
			lastRealGindex := levelBaseGindex + uint64(lastRealIndex)
			if _, ok := collector.siblings[lastRealGindex]; ok {
				collector.siblings[lastRealGindex] = current[lastRealIndex]
			}
		}

		// Hash pairs
		next := make([][32]byte, len(current)/2)
		for i := 0; i < len(current); i += 2 {
			left := current[i]
			right := current[i+1]
			next[i/2] = sha256Two(left, right)

			// Skip the last pair if we already handled it in the odd case above
			if oddNodeLength && i == len(current)-2 {
				continue
			}

			// Check if left or right node is a sibling we need to collect
			leftGindex := levelBaseGindex + uint64(i)
			rightGindex := levelBaseGindex + uint64(i+1)

			if _, ok := collector.siblings[leftGindex]; ok {
				collector.siblings[leftGindex] = left
			}
			if _, ok := collector.siblings[rightGindex]; ok {
				collector.siblings[rightGindex] = right
			}
		}
		current = next
	}

	return current[0]
}

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

// sha256Two hashes two 32-byte chunks together.
// avoiding allocating a new hasher per call: hash a fixed 64-byte buffer.
func sha256Two(left, right [32]byte) [32]byte {
	var buf [64]byte
	copy(buf[:32], left[:])
	copy(buf[32:], right[:])
	return sha256.Sum256(buf[:])
}

// putLittleEndian writes an unsigned integer value in little-endian format.
// Supports sizes 1, 2, 4, or 8 bytes for uint8/16/32/64 respectively.
func putLittleEndian(dst []byte, val uint64, size int) {
	for i := 0; i < size; i++ {
		dst[i] = byte(val >> (8 * i))
	}
}

// applyLengthMixin handles the final mix-in layer for list/bitlist:
// root = sha256Two(dataRoot, lengthHash)
// It also updates the collector for siblings/leaves at the length mixin level.
func applyLengthMixin(currentGindex uint64, dataRoot [32]byte, lengthHash [32]byte, collector *ProofCollector) {
	dataRootGindex := currentGindex * 2
	lengthHashGindex := currentGindex*2 + 1

	// Check if dataRoot is a sibling we need to collect
	if _, ok := collector.siblings[dataRootGindex]; ok {
		collector.siblings[dataRootGindex] = dataRoot
	}
	// Check if lengthHash is a sibling we need to collect
	if _, ok := collector.siblings[lengthHashGindex]; ok {
		collector.siblings[lengthHashGindex] = lengthHash
	}

	// Check if dataRoot is a leaf we need to collect
	if _, ok := collector.leaves[dataRootGindex]; ok {
		collector.leaves[dataRootGindex] = dataRoot
	}
	// Check if lengthHash is a leaf we need to collect
	if _, ok := collector.leaves[lengthHashGindex]; ok {
		collector.leaves[lengthHashGindex] = lengthHash
	}
}
