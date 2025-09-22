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
// Returns:
// The method handles all SSZ-supported types including:
// Example:
func HashTreeRoot(si *sszquery.SSZInfo, serializedData []byte) ([32]byte, error) {
	pool := &ssz.DefaultHasherPool

	hh := pool.Get()
	defer func() {
		pool.Put(hh)
	}()

	err := buildRootFromSSZInfo(si, serializedData, hh, false, 0)
	if err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
}

// buildRootFromSSZInfo is the core recursive function for computing hash tree roots of Go values.
//
// Parameters:
// Returns:
// The method handles all SSZ-supported types including:
// Example:
func buildRootFromSSZInfo(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher, isInList bool, listLimit uint64) error {
	// hashIndex := hh.Index()

	if si == nil {
		return nil
	}

	// https://github.com/ethereum/consensus-specs/blob/dev/ssz/simple-serialize.md#typing
	switch si.Type() {
	case sszquery.Boolean, sszquery.UintN, sszquery.Byte:
		hh.PutBytes(serializedData[:si.FixedSize()])
	case sszquery.Vector, sszquery.Bitvector:
		err := buildRootFromVector(si, serializedData, hh, 0)
		if err != nil {
			return err
		}
	case sszquery.List, sszquery.Bitlist, sszquery.ProgressiveList:
		err := buildRootFromList(si, serializedData, hh, 0)
		if err != nil {
			return err
		}
	case sszquery.Union:
		err := buildRootFromCompatibleUnion(si, serializedData, hh, 0)
		if err != nil {
			return err
		}
	case sszquery.Container:
		err := buildRootFromContainer(si, serializedData, hh, 0)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported SSZ type %s", si.Type())
	}
	return nil
}

// buildRootFromVector computes the hash tree root for ssz vectors.
//
// Parameters:
// Returns:
// The method handles all SSZ-supported types including:
// Example:
func buildRootFromVector(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher, idt int) error {
	hashIndex := hh.Index()

	if si == nil {
		return fmt.Errorf("nil SSZInfo")
	}
	if si.Type() != sszquery.Vector && si.Type() != sszquery.Bitvector {
		return fmt.Errorf("expected vector type, got %s", si.Type())
	}

	vi, err := si.VectorInfo()
	if err != nil {
		return err
	}

	elemType, err := vi.Element()
	if err != nil {
		return err
	}

	vectorLength := vi.Length()
	if vectorLength == 0 {
		// empty vector
		hh.Merkleize(hashIndex)
		return nil
	}

	if isBasicType(elemType.Type()) {
		// merkleize(pack(value)) if value is a basic object or a vector of basic objects.
		// pack(values): Given ordered objects of the same basic type:
		// - Serialize values into bytes. DONE
		// - If not aligned to a multiple of BYTES_PER_CHUNK bytes, right-pad with zeroes to the next multiple.
		// - Partition the bytes into BYTES_PER_CHUNK-byte chunks.
		// - Return the chunks.
		// PutBytes handles chunking automatically for data > 32 bytes
		hh.PutBytes(serializedData[:vectorLength*elemType.Size()])
	} else if elemType.Type() == sszquery.Bitvector {
		// pack_bits(bits): Given the bits of bitlist or bitvector, get bitfield_bytes by packing them in bytes and aligning to the start. The length-delimiting bit for bitlists is excluded. Then return pack(bitfield_bytes).
		// merkleize(pack_bits(value), limit=chunk_count(type)) if value is a bitvector.
		hh.PutBytes(serializedData[:(vectorLength+7)/8])
	} else {
		// merkleize([hash_tree_root(element) for element in value]) if value is a vector of composite objects or a container.
		// // For composed types, hash each element individually
		// for i := 0; (i) < int(vectorLength); i++ {
		// 	err := buildRootFromSSZInfo(elemType, serializedData[i*elemType.Size():(i+1)*elemType.Size()], hh, true, 0)
		// 	if err != nil {
		// 		return fmt.Errorf("failed to hash vector element %d: %w", i, err)
		// 	}
		// }
	}
	hh.Merkleize(hashIndex)
	return nil
}

// buildRootFromList computes the hash tree root for ssz lists.
//
// Parameters:
// Returns:
// The method handles all SSZ-supported types including:
// Example:
func buildRootFromList(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher, idt int) error {
	hashIndex := hh.Index()

	if si == nil {
		return nil
	}
	if si.Type() != sszquery.List && si.Type() != sszquery.Bitlist && si.Type() != sszquery.ProgressiveList {
		return fmt.Errorf("expected list type, got %s", si.Type())
	}

	li, err := si.ListInfo()
	if err != nil {
		return err
	}

	elemType, err := li.Element()
	if err != nil {
		return err
	}

	listLimit := li.Limit()
	if listLimit == 0 {
		return fmt.Errorf("list limit is zero")
	}

	listLength := li.Length()
	if listLength == 0 {
		// empty list
		hh.Merkleize(hashIndex)
		return nil
	}

	offset := si.FixedSize()

	if isBasicType(si.Type()) {
		// pack(values): Given ordered objects of the same basic type:
		// 	- Serialize values into bytes.
		// 	- If not aligned to a multiple of BYTES_PER_CHUNK bytes, right-pad with zeroes to the next multiple.
		// 	- Partition the bytes into BYTES_PER_CHUNK-byte chunks.
		// 	- Return the chunks.
		// merkleize(pack(value)) if value is a basic object or a vector of basic objects.
		// mix_in_length(merkleize(pack(value), limit=chunk_count(type)), len(value)) if value is a list of basic objects.
		// mix_in_length: Given a Merkle root and a length ("uint256" little-endian serialization) return hash(root + length).
		// PutBytes handles chunking automatically for data > 32 bytes
		hh.PutBytes(serializedData[offset : offset+listLength*elemType.Size()])
		hh.MerkleizeWithMixin(hashIndex, listLength, (listLimit*elemType.Size()+31)/32)
	} else if si.Type() == sszquery.Bitlist {
		// mix_in_length(merkleize(pack_bits(value), limit=chunk_count(type)), len(value)) if value is a bitlist.
		// pack_bits(bits): Given the bits of bitlist or bitvector, get bitfield_bytes by packing them in bytes and aligning to the start. The length-delimiting bit for bitlists is excluded. Then return pack(bitfield_bytes).
		// PutBitlist handles length-delimiting bit removal and proper length mixing
		hh.PutBitlist(serializedData[offset:], listLimit)
	} else {
		// mix_in_length(merkleize([hash_tree_root(element) for element in value], limit=chunk_count(type)), len(value)) if value is a list of composite objects.
	}

	return nil
}

// buildRootFromCompatibleUnion computes the hash tree root for CompatibleUnion values.
//
// Parameters:
// Returns:
// The method handles all SSZ-supported types including:
// Example:
func buildRootFromCompatibleUnion(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher, idt int) error {

	return nil
}

// buildRootFromContainer computes the hash tree root for ssz containers.
//
// Parameters:
// Returns:
// The method handles all SSZ-supported types including:
// Example:
func buildRootFromContainer(si *sszquery.SSZInfo, serializedData []byte, hh *ssz.Hasher, idt int) error {
	hashIndex := hh.Index()

	if si == nil {
		return nil
	}
	if si.Type() != sszquery.Container {
		return fmt.Errorf("expected container type, got %s", si.Type())
	}

	ci, err := si.ContainerInfo()
	if err != nil {
		return err
	}

	for _, fieldName := range ci.Order() {
		fieldInfo, ok := ci.Fields()[fieldName]
		if !ok {
			return fmt.Errorf("field %s not found in container fields", fieldName)
		}

		fieldType := fieldInfo.SSZ()
		if fieldType == nil {
			return fmt.Errorf("field %s has nil SSZInfo", fieldName)
		}

		fieldOffset := fieldInfo.Offset()
		fieldSize := fieldType.Size()
		if fieldSize == 0 {
			return fmt.Errorf("field %s has zero size", fieldName)
		}

		err := buildRootFromSSZInfo(fieldType, serializedData[fieldOffset:fieldOffset+fieldSize], hh, false, 0)
		if err != nil {
			return fmt.Errorf("failed to hash container field %s: %w", fieldName, err)
		}
	}

	hh.Merkleize(hashIndex)

	return nil
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
