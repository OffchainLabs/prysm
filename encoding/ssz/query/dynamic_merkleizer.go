package query

import (
	"fmt"
	"reflect"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz/query/proof"
)

// HashTreeRootWith builds a complete merkle tree for any SSZObject by walking its structure
// and hashing all fields using the provided HashWalker. Pre-analyzed SszInfo provides metadata
// (field order, max sizes, element types) to guide the reflection-based traversal.
//
// If sszInfo is nil, the object will be analyzed on-the-fly, but providing pre-analyzed
// SszInfo avoids redundant reflection overhead when the same type is processed multiple times.
func HashTreeRootWith(object any, info *SszInfo, hh proof.HashWalker) error {
	var sszInfo *SszInfo
	var err error

	if info != nil {
		sszInfo = info
	} else {
		// Analyze the object to get pre-computed SSZ type information including max sizes
		sszObj, ok := object.(SSZObject)
		if !ok {
			return fmt.Errorf("object does not implement SSZObject interface")
		}

		sszInfo, err = AnalyzeObject(sszObj)
		if err != nil {
			return fmt.Errorf("failed to analyze object: %w", err)
		}
	}

	sourceValue := reflect.ValueOf(object)
	// Dereference pointers
	if sourceValue.Kind() == reflect.Ptr {
		if sourceValue.IsNil() {
			return fmt.Errorf("cannot build merkle tree from nil object")
		}
		sourceValue = sourceValue.Elem()
	}

	// Start with pack=false for the root container
	return buildRootFromType(sszInfo, sourceValue, hh, false)
}

// buildRootFromType is the main dispatcher that routes to appropriate handlers based on SszInfo type.
// Uses pre-analyzed SSZ type information (Container, List, Vector, etc.) for accurate handling
// and access to metadata like max sizes. The 'pack' parameter indicates whether to use
// Append* (true) or Put* (false) methods.
func buildRootFromType(info *SszInfo, sourceValue reflect.Value, hh proof.HashWalker, pack bool) error {
	if info == nil {
		return fmt.Errorf("SszInfo is nil")
	}

	// Dereference pointers
	for sourceValue.Kind() == reflect.Ptr {
		if sourceValue.IsNil() {
			return fmt.Errorf("nil pointer")
		}
		sourceValue = sourceValue.Elem()
	}

	switch info.sszType {
	case Container:
		return buildRootFromContainer(info, sourceValue, hh)

	case Vector:
		return buildRootFromVector(info, sourceValue, hh)

	case List:
		return buildRootFromList(info, sourceValue, hh)

	case Uint8:
		return buildRootFromUint8(sourceValue, hh, pack)

	case Uint16:
		return buildRootFromUint16(sourceValue, hh, pack)

	case Uint32:
		return buildRootFromUint32(sourceValue, hh, pack)

	case Uint64:
		return buildRootFromUint64(sourceValue, hh, pack)

	case Boolean:
		return buildRootFromBoolean(sourceValue, hh, pack)

	case Bitvector:
		return buildRootFromBitvector(info, sourceValue, hh)

	case Bitlist:
		return buildRootFromBitlist(info, sourceValue, hh)

	default:
		return fmt.Errorf("unsupported SSZ type: %v", info.sszType)
	}
}

// buildRootFromContainer processes a struct (container in SSZ) field-by-field using reflection.
// Each field is recursively hashed using SszInfo for type guidance, and then the fields are
// merkleized together.
func buildRootFromContainer(info *SszInfo, sourceValue reflect.Value, hh proof.HashWalker) error {
	if sourceValue.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct, got %v", sourceValue.Kind())
	}

	hashIndex := hh.Index()

	// Get container info for field order and metadata
	containerInfo, err := info.ContainerInfo()
	if err != nil {
		return fmt.Errorf("failed to get container info: %w", err)
	}

	// Walk all fields in the container in the order specified
	for _, fieldName := range containerInfo.order {
		fieldInfoData, exists := containerInfo.fields[fieldName]
		if !exists {
			continue
		}

		fieldValue := sourceValue.FieldByName(fieldInfoData.goFieldName)

		// Recursively hash this field using its SszInfo - containers always use pack=false
		err = buildRootFromType(fieldInfoData.sszInfo, fieldValue, hh, false)
		if err != nil {
			return fmt.Errorf("failed to hash field %s: %w", fieldInfoData.goFieldName, err)
		}
	}

	// Merkleize all the fields into a single container root
	hh.Merkleize(hashIndex)

	return nil
}

// buildRootFromVector processes a fixed-size array (vector in SSZ) using SszInfo for element type guidance.
// Byte vectors are handled specially (chunked into 32-byte segments).
// Other arrays have each element processed individually with packing.
// Note: Proto-generated structs use []byte (slices) for fixed-size byte vectors,
func buildRootFromVector(info *SszInfo, sourceValue reflect.Value, hh proof.HashWalker) error {
	// Handle both array and slice types (proto uses slices for byte vectors)
	if sourceValue.Kind() != reflect.Slice {
		return fmt.Errorf("expected array or slice for vector, got %v", sourceValue.Kind())
	}

	hashIndex := hh.Index()
	vectorInfo, err := info.VectorInfo()
	if err != nil {
		return fmt.Errorf("failed to get vector info: %w", err)
	}

	elemInfo, err := vectorInfo.Element()
	if err != nil {
		return fmt.Errorf("failed to get element info: %w", err)
	}

	vectorLength := sourceValue.Len()

	// Special case: byte vectors [N]byte are treated as a single value, chunked
	if elemInfo.sszType == Uint8 {
		// Convert array to byte slice
		byteSlice := make([]byte, vectorLength)
		for i := 0; i < vectorLength; i++ {
			byteSlice[i] = byte(sourceValue.Index(i).Uint())
		}
		hh.AppendBytes32(byteSlice)
	} else {
		// Other array types: process each element with pack=true
		for i := 0; i < vectorLength; i++ {
			elemValue := sourceValue.Index(i)
			err := buildRootFromType(elemInfo, elemValue, hh, true)
			if err != nil {
				return err
			}
		}
		hh.FillUpTo32()
	}

	hh.Merkleize(hashIndex)

	return nil
}

// buildRootFromList processes a dynamic slice (list in SSZ) using SszInfo for element type guidance.
// Uses listInfo.Limit() to get the maximum size for proper length mixing in MerkleizeWithMixin.
// Byte lists are handled specially (chunked into 32-byte segments).
func buildRootFromList(info *SszInfo, sourceValue reflect.Value, hh proof.HashWalker) error {
	if sourceValue.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice, got %v", sourceValue.Kind())
	}

	hashIndex := hh.Index()
	listInfo, err := info.ListInfo()
	if err != nil {
		return fmt.Errorf("failed to get list info: %w", err)
	}

	elemInfo, err := listInfo.Element()
	if err != nil {
		return fmt.Errorf("failed to get element info: %w", err)
	}

	listLength := sourceValue.Len() // Use actual slice length, not analyzed length
	limit := listInfo.Limit()

	// Special case: byte lists/slices []byte (fixed-size Uint8 elements)
	if elemInfo.sszType == Uint8 {
		// Get the byte slice directly
		byteSlice := make([]byte, listLength)
		for i := 0; i < listLength; i++ {
			byteSlice[i] = byte(sourceValue.Index(i).Uint())
		}
		hh.AppendBytes32(byteSlice)
	} else if elemInfo.isVariable {
		// Variable-sized elements: process each element without packing/filling
		// Each element is complete and will be merkleized separately
		for i := 0; i < listLength; i++ {
			elemValue := sourceValue.Index(i)
			err := buildRootFromType(elemInfo, elemValue, hh, false)
			if err != nil {
				return err
			}
		}
	} else {
		// Fixed-size elements: pack them together with padding
		for i := 0; i < listLength; i++ {
			elemValue := sourceValue.Index(i)
			err := buildRootFromType(elemInfo, elemValue, hh, true)
			if err != nil {
				return err
			}
		}
		hh.FillUpTo32()
	}

	// For lists, merkleize with mixin to include length in the hash.
	// Use CalculateLimit to compute the tree capacity, same as fastssz does:
	// - For primitive types (Uint8/16/32/64, Boolean): treeCapacity = (maxElements * elementSize + 31) / 32
	// - For containers/lists/vectors: treeCapacity = maxElements (each element is a separate node)
	var treeCapacity uint64

	if elemInfo.sszType.isBasic() {
		// Primitive types: calculate bytes and convert to chunks
		elementSize := elemInfo.Size()
		if elementSize > 0 {
			maxBytes := limit * elementSize
			treeCapacity = (maxBytes + 31) / 32
		} else {
			treeCapacity = limit
		}
	} else {
		// Variable-size and complex types: each is a separate node, use limit directly
		treeCapacity = limit
	}
	hh.MerkleizeWithMixin(hashIndex, uint64(listLength), treeCapacity)

	return nil
}

// buildRootFromBitvector processes a fixed-size bitvector using SszInfo for type guidance.
// Bitvectors are represented as byte slices in proto, with a fixed bit length.
// They are hashed like regular byte vectors since their size is fixed.
func buildRootFromBitvector(_ *SszInfo, sourceValue reflect.Value, hh proof.HashWalker) error {
	if sourceValue.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice for bitvector, got %v", sourceValue.Kind())
	}

	// Get the byte slice representation of the bitvector
	byteSlice := make([]byte, sourceValue.Len())
	for i := 0; i < sourceValue.Len(); i++ {
		byteSlice[i] = byte(sourceValue.Index(i).Uint())
	}

	// Bitvectors are fixed-size byte sequences, hashed the same way as byte vectors
	// PutBytes handles merkleization internally if needed
	hh.PutBytes(byteSlice)

	return nil
}

// buildRootFromBitlist processes a dynamic-size bitlist using SszInfo for type guidance.
// Bitlists are represented as byte slices in proto, with a max bit limit and actual length.
func buildRootFromBitlist(info *SszInfo, sourceValue reflect.Value, hh proof.HashWalker) error {
	if sourceValue.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice for bitlist, got %v", sourceValue.Kind())
	}

	bitlistInfo, err := info.BitlistInfo()
	if err != nil {
		return fmt.Errorf("failed to get bitlist info: %w", err)
	}

	// Get the byte slice representation of the bitlist
	byteSlice := make([]byte, sourceValue.Len())
	for i := 0; i < sourceValue.Len(); i++ {
		byteSlice[i] = byte(sourceValue.Index(i).Uint())
	}

	// For bitlists, use PutBitlist which handles the length encoding and merkleization
	hh.PutBitlist(byteSlice, bitlistInfo.Limit())

	return nil
}

// buildRootFromUint8 processes an 8-bit unsigned integer using Append or Put based on pack parameter.
func buildRootFromUint8(sourceValue reflect.Value, hh proof.HashWalker, pack bool) error {
	val := uint8(sourceValue.Uint())
	if pack {
		hh.AppendUint8(val)
	} else {
		hh.PutUint8(val)
	}
	return nil
}

// buildRootFromUint16 processes a 16-bit unsigned integer using Append or Put based on pack parameter.
func buildRootFromUint16(sourceValue reflect.Value, hh proof.HashWalker, pack bool) error {
	val := uint16(sourceValue.Uint())
	if pack {
		hh.AppendUint16(val)
	} else {
		hh.PutUint16(val)
	}
	return nil
}

// buildRootFromUint32 processes a 32-bit unsigned integer using Append or Put based on pack parameter.
func buildRootFromUint32(sourceValue reflect.Value, hh proof.HashWalker, pack bool) error {
	val := uint32(sourceValue.Uint())
	if pack {
		hh.AppendUint32(val)
	} else {
		hh.PutUint32(val)
	}
	return nil
}

// buildRootFromUint64 processes a 64-bit unsigned integer using Append or Put based on pack parameter.
func buildRootFromUint64(sourceValue reflect.Value, hh proof.HashWalker, pack bool) error {
	val := sourceValue.Uint()
	if pack {
		hh.AppendUint64(val)
	} else {
		hh.PutUint64(val)
	}
	return nil
}

// buildRootFromBoolean processes a boolean value. Only Put is supported; Append is not available in HashWalker.
func buildRootFromBoolean(sourceValue reflect.Value, hh proof.HashWalker, pack bool) error {
	if pack {
		return fmt.Errorf("AppendBool not available in HashWalker")
	}
	val := sourceValue.Bool()
	hh.PutBool(val)
	return nil
}
