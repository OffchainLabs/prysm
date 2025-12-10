package query

import (
	"fmt"
	"reflect"

	"github.com/OffchainLabs/go-bitfield"
	ssz "github.com/prysmaticlabs/fastssz"
)

// MerkleTree builds and returns the full SSZ Merkle tree for the analyzed object.
// The returned Node can be used to compute the hash tree root via Node.Hash(),
// or to generate inclusion proofs via Node.Prove(gindex).
func (info *SszInfo) MerkleTree() (*ssz.Node, error) {
	return info.toMerkleTree(&ssz.Wrapper{})
}

// toMerkleTree constructs an SSZ Merkle tree using fastssz's Wrapper.
// It is the entrypoint for traversing the analyzed SSZ structure and
// building the tree by adding leaves and committing subtrees according
// to SSZ merkleization rules.
func (info *SszInfo) toMerkleTree(w *ssz.Wrapper) (*ssz.Node, error) {
	if info == nil {
		return nil, fmt.Errorf("nil SszInfo")
	}

	// info.source is guaranteed to be valid and dereferenced by AnalyzeObject
	v := reflect.ValueOf(info.source).Elem()

	if err := buildTree(info, v, w); err != nil {
		return nil, err
	}

	return w.Node(), nil
}

// buildTree recursively merkleizes a value according to SSZ rules.
// It dispatches to type-specific handlers based on the SSZ type
func buildTree(info *SszInfo, v reflect.Value, w *ssz.Wrapper) error {
	if info.sszType.isBasic() {
		return addLeafFromBasicType(info.sszType, v, w)
	}
	switch info.sszType {
	case Container:
		return buildContainerSubtree(info, v, w)
	case List:
		return buildListSubtree(info, v, w)
	case Vector:
		return buildVectorSubtree(info, v, w)
	case Bitlist:
		return buildBitlistSubtree(info, v, w)
	case Bitvector:
		return buildBitvectorSubtree(info, v, w)
	default:
		return fmt.Errorf("unsupported SSZ type: %v", info.sszType)
	}
}

// addLeafFromBasicType adds a single leaf node for a basic SSZ type.
func addLeafFromBasicType(t SSZType, v reflect.Value, w *ssz.Wrapper) error {
	switch t {
	case Uint8:
		w.AddUint8(uint8(v.Uint())) // reflect.Uint returns uint64, safe to cast
	case Uint16:
		w.AddUint16(uint16(v.Uint()))
	case Uint32:
		w.AddUint32(uint32(v.Uint()))
	case Uint64:
		w.AddUint64(v.Uint())
	case Boolean:
		if v.Bool() {
			w.AddUint8(1)
		} else {
			w.AddUint8(0)
		}
	default:
		return fmt.Errorf("unexpected basic type: %v", t)
	}
	return nil
}

// buildContainerSubtree merkleizes a container by processing each field in order.
// Fields are padded to the next power of 2 before committing the subtree.
func buildContainerSubtree(info *SszInfo, v reflect.Value, w *ssz.Wrapper) error {
	ci, err := info.ContainerInfo()
	if err != nil {
		return err
	}

	// Dereference pointer if needed
	v = dereferencePointer(v)

	start := w.Indx()
	for _, name := range ci.order {
		fieldInfo := ci.fields[name]
		fieldVal := v.FieldByName(fieldInfo.goFieldName)
		if err := buildTree(fieldInfo.sszInfo, fieldVal, w); err != nil {
			return fmt.Errorf("field %s: %w", name, err)
		}
	}

	// Container commits require power-of-2 padding
	addPadding(w, len(ci.order))
	w.Commit(start)

	return nil
}

// buildVectorSubtree merkleizes a fixed-length sequence of elements.
// Basic types are packed into 32-byte chunks; composite types are processed recursively.
// Elements are padded to the next power of 2 before committing.
func buildVectorSubtree(info *SszInfo, v reflect.Value, w *ssz.Wrapper) error {
	vi, err := info.VectorInfo()
	if err != nil {
		return err
	}

	start := w.Indx()
	length := v.Len()

	if vi.element.sszType.isBasic() {
		// Vectors of basic types are packed into 32-byte chunks
		length = packBasicElements(vi.element, v, w, length)
	} else {
		// General case: vector of composite elements (each element becomes its own subtree)
		for i := 0; i < length; i++ {
			if err := buildTree(vi.element, v.Index(i), w); err != nil {
				return fmt.Errorf("vector index %d: %w", i, err)
			}
		}
	}

	addPadding(w, length)
	w.Commit(start)
	return nil
}

// buildListSubtree merkleizes a variable-length sequence of elements.
// Basic types are packed into 32-byte chunks; composite types are processed recursively.
// The length is mixed into the root hash via CommitWithMixin.
func buildListSubtree(info *SszInfo, v reflect.Value, w *ssz.Wrapper) error {
	li, err := info.ListInfo()
	if err != nil {
		return err
	}

	length := v.Len()
	limit := li.Limit()
	start := w.Indx()
	elemInfo := li.element

	if elemInfo.sszType.isBasic() {
		// Lists of basic types are packed into 32-byte chunks
		packBasicElements(elemInfo, v, w, length)

		limit = ssz.CalculateLimit(limit, uint64(length), uint64(itemLength(elemInfo)))
	} else {
		// General case: list of composite elements
		for i := 0; i < length; i++ {
			if err := buildTree(elemInfo, v.Index(i), w); err != nil {
				return fmt.Errorf("list index %d: %w", i, err)
			}
		}
	}

	w.CommitWithMixin(start, length, int(nextPowerOfTwo(uint64(limit))))
	return nil
}

// buildBitlistSubtree merkleizes a variable-length bit sequence.
// The termination bit is cleared before chunking, and the bit length
// is mixed into the root hash via CommitWithMixin.
func buildBitlistSubtree(info *SszInfo, v reflect.Value, w *ssz.Wrapper) error {
	bi, err := info.BitlistInfo()
	if err != nil {
		return err
	}

	bitlistBytes := v.Bytes()

	// Handle zero-initialized bitlist: create a single byte with just the termination bit
	if len(bitlistBytes) == 0 {
		bitlistBytes = []byte{0x01}
	}

	// Use go-bitfield to get length and bytes with termination bit cleared
	bl := bitfield.Bitlist(bitlistBytes)
	data := bl.BytesNoTrim()
	start := w.Indx()

	// Add bytes in 32-byte chunks
	for i := 0; i < len(data); i += 32 {
		end := i + 32
		if end > len(data) {
			end = len(data)
		}
		w.AddBytes(data[i:end])
	}

	// Number of 32-byte chunks to cover the max bit capacity.
	// In consensus specs these max bit sizes are chosen so this is a power of 2.
	limitChunks := int((bi.limit + 255) / 256)

	w.CommitWithMixin(start, int(bi.length), limitChunks)
	return nil
}

// buildBitvectorSubtree merkleizes a fixed-length bit sequence.
// Bytes are chunked into 32-byte leaves and padded to the next power of 2.
// Unlike bitlists, no length mixin is applied since the size is fixed.
func buildBitvectorSubtree(info *SszInfo, v reflect.Value, w *ssz.Wrapper) error {
	bv, err := info.BitvectorInfo()
	if err != nil {
		return err
	}

	bitvectorBytes := v.Bytes()

	// bitvector must be initialized and non-empty
	if len(bitvectorBytes) == 0 {
		return fmt.Errorf("bitvector field is uninitialized (nil or empty slice)")
	}

	start := w.Indx()
	// Add bytes in 32-byte chunks
	length := bv.Length()
	numChunks := int(length / 32)
	for i := 0; i < len(bitvectorBytes); i += 32 {
		end := i + 32
		if end > len(bitvectorBytes) {
			end = len(bitvectorBytes)
		}
		chunk := bitvectorBytes[i:end]
		w.AddBytes(chunk)
	}
	addPadding(w, numChunks) // fixed-size, no mixin
	w.Commit(start)

	return nil
}

// packBasicElements packs basic type elements into 32-byte chunks and adds them to the wrapper.
// Returns the number of chunks written (used by vectors to determine padding).
func packBasicElements(elemInfo *SszInfo, v reflect.Value, w *ssz.Wrapper, length int) int {
	if length == 0 {
		return 0
	}

	elemSize := int(itemLength(elemInfo))
	elemsPerChunk := 32 / elemSize
	numChunks := (length + elemsPerChunk - 1) / elemsPerChunk

	for chunkIdx := 0; chunkIdx < numChunks; chunkIdx++ {
		chunk := make([]byte, 32)
		for i := 0; i < elemsPerChunk; i++ {
			elemIdx := chunkIdx*elemsPerChunk + i
			if elemIdx >= length {
				break
			}
			offset := i * elemSize
			if elemInfo.sszType == Boolean {
				if v.Index(elemIdx).Bool() {
					chunk[offset] = 1
				}
			} else {
				putLittleEndian(chunk[offset:], v.Index(elemIdx).Uint(), elemSize)
			}
		}
		w.AddBytes(chunk)
	}

	return numChunks
}

// addPadding adds empty nodes to reach the next power of 2,
// ensuring proper binary tree structure for SSZ merkleization.
func addPadding(w *ssz.Wrapper, count int) {
	paddedLength := int(nextPowerOfTwo(uint64(count)))
	for i := count; i < paddedLength; i++ {
		w.AddEmpty()
	}
}

// putLittleEndian writes an unsigned integer value in little-endian format.
// Supports sizes 1, 2, 4, or 8 bytes for uint8/16/32/64 respectively.
func putLittleEndian(dst []byte, val uint64, size int) {
	for i := 0; i < size; i++ {
		dst[i] = byte(val >> (8 * i))
	}
}
