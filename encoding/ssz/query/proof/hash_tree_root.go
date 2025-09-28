package proof

import (
	"fmt"

	sszquery "github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	ssz "github.com/prysmaticlabs/fastssz"
)

// HashTreeRoot computes the hash tree root according to the SSZ spec for any given SSZInfo object + the serialized data.
//
// The hash tree root is a cryptographic commitment to the entire data structure, used extensively
// in Ethereum's consensus layer for creating Merkle proofs and maintaining state roots. This method
// implements the SSZ hash tree root algorithm, which recursively hashes all fields and combines
// them using binary Merkle trees.
//
// Parameters:
// - si: The SSZInfo describing the structure of the data.
// - serializedData: The SSZ-serialized byte slice of the object to compute the hash tree root for.
// Returns:
// - 32-byte hash tree root of the object.
// - error if any issues occur during computation.
// The method handles all SSZ-supported types including:
func HashTreeRoot(si *sszquery.SSZInfo, serializedData []byte) ([32]byte, error) {
	pool := &ssz.DefaultHasherPool

	hh := pool.Get()
	defer func() {
		pool.Put(hh)
	}()

	err := buildRootFromSSZInfo(si, serializedData, hh)
	if err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
}

// buildRootFromSSZInfo is the core recursive function for computing hash tree roots of Go values.
//
// Parameters:
// - si: The SSZInfo describing the structure of the data.
// - serializedData: The SSZ-serialized byte slice of the object to compute the hash tree root for.
// - hh: The hasher instance to use for computing the hash tree root.
// Returns:
// - error if any issues occur during computation.
// The method handles all SSZ-supported types including:
func buildRootFromSSZInfo(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher) error {
	if si == nil {
		return fmt.Errorf("buildRootFromSSZInfo: SSZInfo cannot be nil")
	}

	if hh == nil {
		return fmt.Errorf("buildRootFromSSZInfo: hasher cannot be nil")
	}

	if serializedData == nil {
		return fmt.Errorf("buildRootFromSSZInfo: serializedData cannot be nil")
	}

	// https://github.com/ethereum/consensus-specs/blob/dev/ssz/simple-serialize.md#typing
	switch si.Type() {
	case sszquery.Boolean, sszquery.UintN, sszquery.Byte:
		err := buildRootFromBasicType(si, serializedData, hh)
		if err != nil {
			return err
		}
	case sszquery.Vector, sszquery.Bitvector:
		err := buildRootFromVector(si, serializedData, hh)
		if err != nil {
			return err
		}
	case sszquery.List, sszquery.Bitlist, sszquery.ProgressiveList:
		err := buildRootFromList(si, serializedData, hh)
		if err != nil {
			return err
		}
	case sszquery.Union:
		err := buildRootFromCompatibleUnion(si, serializedData, hh)
		if err != nil {
			return err
		}
	case sszquery.Container:
		err := buildRootFromContainer(si, serializedData, hh)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("buildRootFromSSZInfo: unsupported SSZ type %s, expected one of: Boolean, UintN, Byte, Vector, Bitvector, List, Bitlist, ProgressiveList, Union, Container", si.Type())
	}
	return nil
}

// buildRootFromBasicType computes the hash tree root for basic SSZ types (boolean, uintN, byte).
//
// Parameters:
// - si: The SSZInfo describing the structure of the data.
// - serializedData: The SSZ-serialized byte slice of the object to compute the hash tree root for.
// - hh: The hasher instance to use for computing the hash tree root.
// Returns:
// - error if any issues occur during computation.
func buildRootFromBasicType(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher) error {
	if hh == nil {
		return fmt.Errorf("buildRootFromBasicType: hasher cannot be nil")
	}

	fixedSize := si.FixedSize()
	if uint64(len(serializedData)) < fixedSize {
		return fmt.Errorf("buildRootFromBasicType: insufficient data for %s type, need %d bytes but have %d bytes", si.Type(), fixedSize, len(serializedData))
	}

	hashIndex := hh.Index()
	hh.PutBytes(serializedData[:fixedSize])
	hh.Merkleize(hashIndex)
	return nil
}

// buildRootFromVector computes the hash tree root for ssz vectors.
//
// Parameters:
// - si: The SSZInfo describing the structure of the data.
// - serializedData: The SSZ-serialized byte slice of the object to compute the hash tree root for.
// - hh: The hasher instance to use for computing the hash tree root.
// Returns:
// - error if any issues occur during computation.
func buildRootFromVector(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher) error {
	hashIndex := hh.Index()

	if si.Type() != sszquery.Vector && si.Type() != sszquery.Bitvector {
		return fmt.Errorf("buildRootFromVector: expected Vector or Bitvector type, got %s", si.Type())
	}

	if si.Type() == sszquery.Bitvector {
		fixedSize := si.FixedSize()
		if uint64(len(serializedData)) < fixedSize {
			return fmt.Errorf("buildRootFromVector: insufficient data for Bitvector, need %d bytes but have %d bytes", fixedSize, len(serializedData))
		}
		// Pack bits into bytes and merkleize for bitvector hash tree root
		hh.PutBytes(serializedData[:fixedSize])
		hh.Merkleize(hashIndex)
		return nil
	}

	vi, err := si.VectorInfo()
	if err != nil {
		return fmt.Errorf("buildRootFromVector: failed to get vector info: %w", err)
	}

	elemType, err := vi.Element()
	if err != nil {
		return fmt.Errorf("buildRootFromVector: failed to get element type info: %w", err)
	}

	vectorLength := vi.Length() // NOTE: vectorLength cannot be zero for valid SSZ vectors

	if isBasicType(elemType.Type()) {
		requiredSize := vectorLength * elemType.Size()
		if uint64(len(serializedData)) < requiredSize {
			return fmt.Errorf("buildRootFromVector: insufficient data for Vector[%s, %d], need %d bytes but have %d bytes", elemType.Type(), vectorLength, requiredSize, len(serializedData))
		}
		// Pack basic type elements into bytes and merkleize for vector hash tree root
		hh.PutBytes(serializedData[:requiredSize])
	} else {
		// Hash each composite element individually, then merkleize all hashes
		elemSize := elemType.Size()
		// Validate element size to prevent potential issues
		if elemSize == 0 {
			return fmt.Errorf("buildRootFromVector: element type %s has zero size, cannot process vector elements", elemType.Type())
		}

		// Check if we have enough data for all vector elements
		requiredDataSize := vectorLength * elemSize
		if uint64(len(serializedData)) < requiredDataSize {
			return fmt.Errorf("buildRootFromVector: insufficient data for Vector[%s, %d], need %d bytes (elements) but have %d bytes", elemType.Type(), vectorLength, requiredDataSize, len(serializedData))
		}

		for i := uint64(0); i < vectorLength; i++ {
			elementOffset := i * elemSize
			elementData := serializedData[elementOffset : elementOffset+elemSize]

			err := buildRootFromSSZInfo(elemType, elementData, hh)
			if err != nil {
				return fmt.Errorf("buildRootFromVector: failed to hash vector element %d of type %s: %w", i, elemType.Type(), err)
			}
		}
	}
	hh.Merkleize(hashIndex)
	return nil
}

// buildRootFromList computes the hash tree root for ssz lists.
//
// Parameters:
// - si: The SSZInfo describing the structure of the data.
// - serializedData: The SSZ-serialized byte slice of the object to compute the hash tree root for.
// - hh: The hasher instance to use for computing the hash tree root.
// Returns:
// - error if any issues occur during computation.
func buildRootFromList(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher) error {
	hashIndex := hh.Index()

	if si.Type() != sszquery.List && si.Type() != sszquery.Bitlist {
		if si.Type() == sszquery.ProgressiveList {
			return fmt.Errorf("buildRootFromList: ProgressiveList hash tree root computation is not yet implemented")
		} else {
			return fmt.Errorf("buildRootFromList: expected List or Bitlist type, got %s", si.Type())
		}
	}

	if si.Type() == sszquery.Bitlist {
		bi, err := si.BitlistInfo()
		if err != nil {
			return fmt.Errorf("buildRootFromList: failed to get bitlist info: %w", err)
		}

		bitlistLimit := bi.Limit()
		bitlistLength := bi.Length()

		if bitlistLimit == 0 {
			return fmt.Errorf("buildRootFromList: invalid bitlist configuration, limit cannot be zero")
		}

		if len(serializedData) == 0 {
			return fmt.Errorf("buildRootFromList: empty serialized data for bitlist with length %d", bitlistLength)
		}

		hh.PutBitlist(serializedData, bitlistLimit)
		return nil
	}

	li, err := si.ListInfo()
	if err != nil {
		return fmt.Errorf("buildRootFromList: failed to get list info: %w", err)
	}

	elemType, err := li.Element()
	if err != nil {
		return fmt.Errorf("buildRootFromList: failed to get element type info: %w", err)
	}

	listLimit := li.Limit()
	if listLimit == 0 {
		return fmt.Errorf("buildRootFromList: invalid list configuration, limit cannot be zero")
	}

	listLength := li.Length()
	if listLength == 0 {
		// empty list - still needs length mixing for proper list hash
		// Calculate chunk limit for consistency
		if isBasicType(elemType.Type()) {
			hh.MerkleizeWithMixin(hashIndex, 0, ssz.CalculateLimit(listLimit, listLength, elemType.Size()))
		} else {
			hh.MerkleizeWithMixin(hashIndex, 0, listLimit)
		}
		return nil
	}

	elemSize := elemType.Size()
	if elemSize == 0 {
		return fmt.Errorf("buildRootFromList: element type %s has zero size, cannot process list elements", elemType.Type())
	}

	requiredDataSize := listLength * elemSize
	if uint64(len(serializedData)) < requiredDataSize {
		return fmt.Errorf("buildRootFromList: insufficient data for List[%s, %d] with %d elements, need %d bytes but have %d bytes", elemType.Type(), listLimit, listLength, requiredDataSize, len(serializedData))
	}

	// serializedData already contains just the list data (offset has been dereferenced by caller)
	// so we start from the beginning of the data

	if isBasicType(elemType.Type()) {
		// pack(values): Given ordered objects of the same basic type:
		// 	- Serialize values into bytes.
		// 	- If not aligned to a multiple of BYTES_PER_CHUNK bytes, right-pad with zeroes to the next multiple.
		// 	- Partition the bytes into BYTES_PER_CHUNK-byte chunks.
		// 	- Return the chunks.
		// merkleize(pack(value)) if value is a basic object or a vector of basic objects.
		// mix_in_length(merkleize(pack(value), limit=chunk_count(type)), len(value)) if value is a list of basic objects.
		// mix_in_length: Given a Merkle root and a length ("uint256" little-endian serialization) return hash(root + length).
		hh.Append(serializedData[:requiredDataSize])

		// For basic types, calculate the maximum number of chunks based on element size
		hh.MerkleizeWithMixin(hashIndex, listLength, ssz.CalculateLimit(listLimit, listLength, elemSize))
	} else {
		// mix_in_length(merkleize([hash_tree_root(element) for element in value], limit=chunk_count(type)), len(value)) if value is a list of composite objects.
		// For composite types, hash each element individually, then merkleize with length mixing
		for i := uint64(0); i < listLength; i++ {
			elementOffset := i * elemSize
			elementData := serializedData[elementOffset : elementOffset+elemSize]

			err := buildRootFromSSZInfo(elemType, elementData, hh)
			if err != nil {
				return fmt.Errorf("buildRootFromList: failed to hash list element %d of type %s: %w", i, elemType.Type(), err)
			}
		}
		// For composite types, each element becomes one chunk after hashing
		hh.MerkleizeWithMixin(hashIndex, listLength, listLimit)
	}

	return nil
}

// buildRootFromCompatibleUnion computes the hash tree root for CompatibleUnion values.
//
// Parameters:
// - si: The SSZInfo describing the structure of the data.
// - serializedData: The SSZ-serialized byte slice of the object to compute the hash tree root for.
// - hh: The hasher instance to use for computing the hash tree root.
// Returns:
// - error indicating that Union types are not yet implemented.
func buildRootFromCompatibleUnion(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher) error {
	return fmt.Errorf("buildRootFromCompatibleUnion: Union type hash tree root computation is not yet implemented for type %s", si.Type())
}

// buildRootFromContainer computes the hash tree root for ssz containers.
//
// Parameters:
// - si: The SSZInfo describing the structure of the data.
// - serializedData: The SSZ-serialized byte slice of the object to compute the hash tree root for.
// - hh: The hasher instance to use for computing the hash tree root.
// Returns:
// - error if any issues occur during computation.
func buildRootFromContainer(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher) error {
	hashIndex := hh.Index()

	if si.Type() != sszquery.Container {
		return fmt.Errorf("buildRootFromContainer: expected Container type, got %s", si.Type())
	}

	ci, err := si.ContainerInfo()
	if err != nil {
		return fmt.Errorf("buildRootFromContainer: failed to get container info: %w", err)
	}

	fieldOrder := ci.Order()
	if len(fieldOrder) == 0 {
		// Empty container - still needs merkleization
		hh.Merkleize(hashIndex)
		return nil
	}

	fields := ci.Fields()
	for _, fieldName := range fieldOrder {
		fieldInfo, ok := fields[fieldName]
		if !ok {
			return fmt.Errorf("buildRootFromContainer: field %s not found in container fields, available fields: %v", fieldName, getFieldNames(fields))
		}

		fieldType := fieldInfo.SSZ()
		if fieldType == nil {
			return fmt.Errorf("buildRootFromContainer: field %s has nil SSZInfo", fieldName)
		}

		fieldOffset := fieldInfo.Offset()
		fieldSize := fieldType.Size()

		// Validate field bounds
		if fieldOffset >= uint64(len(serializedData)) {
			return fmt.Errorf("buildRootFromContainer: field %s offset %d exceeds data length %d", fieldName, fieldOffset, len(serializedData))
		}

		fieldEndOffset := fieldOffset + fieldSize
		if fieldEndOffset > uint64(len(serializedData)) {
			return fmt.Errorf("buildRootFromContainer: field %s (offset: %d, size: %d) exceeds data bounds, need %d bytes but have %d bytes", fieldName, fieldOffset, fieldSize, fieldEndOffset, len(serializedData))
		}

		err := buildRootFromSSZInfo(fieldType, serializedData[fieldOffset:fieldEndOffset], hh)
		if err != nil {
			return fmt.Errorf("buildRootFromContainer: failed to hash container field %s of type %s: %w", fieldName, fieldType.Type(), err)
		}
	}

	hh.Merkleize(hashIndex)
	return nil
}

// getFieldNames extracts field names from a field map for error reporting
func getFieldNames(fields map[string]*sszquery.FieldInfo) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	return names
}

// --- Helpers ---
func isBasicType(t sszquery.SSZType) bool {
	switch t {
	case sszquery.Boolean, sszquery.UintN, sszquery.Byte:
		return true
	default:
		return false
	}
}
