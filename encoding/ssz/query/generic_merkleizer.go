package query

import (
	"fmt"
	"reflect"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz/query/proof"
)

// HashTreeRootWith builds a complete merkle tree by walking the object structure and hashing
// all fields using the provided HashWalker. Pre-analyzed SszInfo provides metadata (field order,
// max sizes, element types) to guide value extraction and hashing.
//
// Parameters:
//   - info: Pre-analyzed SszInfo containing the source object and type metadata
//   - hh: HashWalker for hash computation (e.g., proof.Wrapper for tree building)
//
// Returns:
//   - error: If hashing fails or SszInfo is nil
func HashTreeRootWith(info *SszInfo, hh proof.HashWalker) error {
	// Use provided SszInfo or analyze the object if nil
	if info == nil {
		return fmt.Errorf("SszInfo cannot be nil")
	}

	sourceValue := reflect.ValueOf(info.source)

	// Start with pack=false for the root container
	return buildRootFromType(info, sourceValue, hh, false)
}

// buildRootFromType is the main dispatcher that routes to appropriate handlers based on SszInfo type.
// Uses pre-analyzed SSZ type information (Container, List, Vector, etc.) for accurate handling
// and access to metadata like limits. The 'pack' parameter indicates whether to use
// Append* (true) or Put* (false) methods.
func buildRootFromType(info *SszInfo, sourceValue reflect.Value, hh proof.HashWalker, pack bool) error {
	if info == nil {
		return fmt.Errorf("SszInfo is nil")
	}

	// Dereference pointers
	if sourceValue.Kind() == reflect.Ptr {
		sourceValue = dereferencePointer(sourceValue)
	}

	switch info.sszType {
	case Container:
		return buildRootFromContainer(info, sourceValue, hh)

	case Vector:
		return buildRootFromVector(info, sourceValue, hh)

	case Bitvector:
		return buildRootFromBitvector(info, sourceValue, hh)

	case List:
		return buildRootFromList(info, sourceValue, hh)

	case Bitlist:
		return buildRootFromBitlist(info, sourceValue, hh)

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

	default:
		return fmt.Errorf("unsupported SSZ type: %v", info.sszType)
	}
}

// buildRootFromContainer computes the hash tree root for ssz containers.
//
// In SSZ, containers are hashed as follows:
//   - Each field is hashed independently to produce a 32-byte root
//   - All field roots are collected in order
//   - The collection is Merkleized to produce the container's root
//
// The function uses pre-analyzed SSZ type information to efficiently iterate through
// fields without repeated reflection calls.
//
// Parameters:
//   - info: The SszInfo containing container field metadata and ordering
//   - sourceValue: The reflect.Value of the container to hash
//   - hh: The HashWalker instance for hash computation
//
// Returns:
//   - error: An error if any field hashing fails
//
// The Merkleize call at the end combines all field hashes into the final root
// using binary tree hashing with zero-padding to the next power of two.
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

// buildRootFromVector computes the hash tree root for ssz vectors.
//
// Arrays in SSZ are hashed based on their element type:
//   - Byte arrays: Treated as a single value, chunked into 32-byte segments
//   - Other arrays: Each element is hashed individually, then Merkleized
//
// For arrays with max size hints, the function uses MerkleizeWithMixin to include
// the array length in the final hash computation.
//
// Parameters:
//   - info: The SszInfo containing vector metadata and max length
//   - sourceValue: The reflect.Value of the array to hash
//   - hh: The HashWalker instance for hash computation
//
// Returns:
//   - error: An error if element hashing fails
//
// Special handling:
//   - Byte arrays use AppendBytes32 for efficient chunk-based hashing
//   - Vectors shorter than max length are padded with zeros
//   - Array types cannot exceed their max length
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

	sliceLen := sourceValue.Len()

	appendZero := 0
	if uint32(sliceLen) < uint32(vectorInfo.Length()) {
		appendZero = int(vectorInfo.Length()) - sliceLen
	}

	// For byte arrays, handle as a single unit
	if elemInfo.sszType == Uint8 {
		if !sourceValue.CanAddr() {
			// workaround for unaddressable static arrays
			sourceValPtr := reflect.New(elemInfo.typ)
			sourceValPtr.Elem().Set(sourceValue)
			sourceValue = sourceValPtr.Elem()
		}

		var bytes []byte
		if elemInfo.typ.Kind() == reflect.String {
			bytes = []byte(sourceValue.String())[:sliceLen]
		} else {
			bytes = sourceValue.Bytes()[:sliceLen]
		}

		if appendZero > 0 {
			zeroBytes := make([]byte, appendZero)
			bytes = append(bytes, zeroBytes...)
		}

		hh.AppendBytes32(bytes)
	} else {
		// For other types, process each element
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)

			err := buildRootFromType(elemInfo, fieldValue, hh, true)
			if err != nil {
				return err
			}
		}

		if appendZero > 0 {
			var zeroVal reflect.Value
			if elemInfo.typ.Kind() == reflect.Ptr {
				zeroVal = reflect.New(elemInfo.typ.Elem())
			} else {
				zeroVal = reflect.New(elemInfo.typ).Elem()
			}

			index := hh.Index()
			err := buildRootFromType(elemInfo, zeroVal, hh, true)
			if err != nil {
				return err
			}

			zeroLen := hh.Index() - index
			zeroBytes := hh.Hash()
			if len(zeroBytes) > zeroLen {
				zeroBytes = zeroBytes[len(zeroBytes)-zeroLen:]
			}

			for i := 1; i < appendZero; i++ {
				hh.Append(zeroBytes)
			}
		}

		hh.FillUpTo32()
	}

	hh.Merkleize(hashIndex)

	return nil
}

// buildRootFromList processes a dynamic slice (list in SSZ) using SszInfo for element type guidance.
// Lists in SSZ are hashed as follows:
//   - Computing the root of the slice contents (as if it were an array)
//   - Mixing the slice length into the final hash for proper domain separation
//
// Uses listInfo.Limit() to get the maximum size for proper length mixing in MerkleizeWithMixin.
// Byte lists are handled specially (chunked into 32-byte segments).
//
// Parameters:
//   - info: The SszInfo containing list element metadata and max limit
//   - sourceValue: The reflect.Value of the slice to hash
//   - hh: The HashWalker instance for hash computation
//
// Returns:
//   - error: An error if element hashing fails or list exceeds limit
//
// Special handling:
//   - Byte lists are appended as a single unit with chunk padding
//   - Variable-sized elements are processed without packing
//   - Fixed-size elements are packed together with padding
//   - List size is mixed into the final hash via MerkleizeWithMixin
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

	sliceLen := sourceValue.Len()
	// NOTE: using listInfo.Length() is only possible if multi-dimensional array has the same length e.g. `[][32]byte`.
	// In case of variable-length multi-dimensional arrays, it cannot be used e.g. `[][]byte` as we only store the length of first element.
	limit := listInfo.Limit()

	// For byte arrays, handle as a single unit
	if elemInfo.sszType == Uint8 {
		if !sourceValue.CanAddr() {
			// workaround for unaddressable static arrays
			sourceValPtr := reflect.New(elemInfo.typ)
			sourceValPtr.Elem().Set(sourceValue)
			sourceValue = sourceValPtr.Elem()
		}

		var bytes []byte
		if elemInfo.typ.Kind() == reflect.String {
			bytes = []byte(sourceValue.String())
		} else {
			bytes = sourceValue.Bytes()
		}

		hh.AppendBytes32(bytes)
	} else {
		// For other types, process each element
		for i := 0; i < int(sliceLen); i++ {
			fieldValue := sourceValue.Index(i)

			err := buildRootFromType(elemInfo, fieldValue, hh, true)
			if err != nil {
				return err
			}
		}

		hh.FillUpTo32()
	}

	// Merkleize with mixin to include length in the hash
	if limit > 0 {
		// Calculate tree capacity (you already do this above)
		var treeCapacity uint64
		if elemInfo.sszType.isBasic() && elemInfo.Size() > 0 {
			treeCapacity = proof.CalculateLimit(limit, uint64(sliceLen), elemInfo.Size())
		} else {
			treeCapacity = limit
		}

		hh.MerkleizeWithMixin(hashIndex, uint64(sliceLen), treeCapacity)

	} else {
		hh.Merkleize(hashIndex)
	}

	return nil
}

// buildRootFromBitvector processes a fixed-size bitvector using SszInfo for type guidance.
// Bitvectors are represented as byte slices in proto, with a fixed bit length.
// Appends bytes as a single unit and merkleizes them.
func buildRootFromBitvector(_ *SszInfo, sourceValue reflect.Value, hh proof.HashWalker) error {
	hashIndex := hh.Index()
	bytes := sourceValue.Bytes()

	// Append bytes in 32-byte chunks
	hh.AppendBytes32(bytes)

	// Merkleize the result
	hh.Merkleize(hashIndex)

	return nil
}

// buildRootFromBitlist processes a dynamic-size bitlist using SszInfo for type guidance.
// Bitlists are represented as byte slices in proto, with a max bit limit and actual length.
// Uses PutBitlist which handles parsing and merkleizing internally.
func buildRootFromBitlist(info *SszInfo, sourceValue reflect.Value, hh proof.HashWalker) error {
	bytes := sourceValue.Bytes()

	bitlistInfo, err := info.BitlistInfo()
	if err != nil {
		return fmt.Errorf("failed to get bitlist info: %w", err)
	}

	// PutBitlist handles parsing and merkleizing with mixin internally
	hh.PutBitlist(bytes, bitlistInfo.Limit())

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
func buildRootFromBoolean(sourceValue reflect.Value, hh proof.HashWalker, _ bool) error {
	val := sourceValue.Bool()
	hh.PutBool(val)
	return nil
}
