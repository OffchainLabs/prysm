# SSZ Query Package

The `encoding/ssz/query` package provides a system for analyzing, querying, and generating Merkle proofs for SSZ ([Simple Serialize](https://github.com/ethereum/consensus-specs/blob/master/ssz/simple-serialize.md)) data structures. It enables runtime analysis of SSZ-serialized Go objects with reflection, path-based queries through nested structures, generalized index calculation, and Merkle proof generation.

This package is designed to be **generic**, it operates on arbitrary SSZ-serialized Go values at runtime, so the same query/proof machinery applies equally to core consensus types like BeaconState/BeaconBlock and to any customized SSZ schemas.

## Usage Example

```go
// 1. Analyze an SSZ object
block := &ethpb.BeaconBlock{...}
info, err := query.AnalyzeObject(block)

// 2. Parse a path
path, err := query.ParsePath(".body.attestations[0].data.slot")

// 3. Get the generalized index
gindex, err := query.GetGeneralizedIndexFromPath(info, path)

// 4. Generate a Merkle proof
proof, err := info.Prove(gindex)

// 5. Get offset and length to slice the SSZ-encoded bytes
sszBytes, _ := block.MarshalSSZ()
_, offset, length, err := query.CalculateOffsetAndLength(info, path)
// slotBytes contains the SSZ-encoded value at the queried path
slotBytes := sszBytes[offset : offset+length]
```

## Exported API

The main exported API consists of:

```go
// AnalyzeObject analyzes an SSZ object and returns its structural information
func AnalyzeObject(obj SSZObject) (*SszInfo, error)

// ParsePath parses a path string like ".field1.field2[0].field3"
func ParsePath(rawPath string) (Path, error)

// CalculateOffsetAndLength computes byte offset and length for a path within an SSZ object
func CalculateOffsetAndLength(sszInfo *SszInfo, path Path) (*SszInfo, uint64, uint64, error)

// GetGeneralizedIndexFromPath calculates the generalized index for a given path
func GetGeneralizedIndexFromPath(info *SszInfo, path Path) (uint64, error)

// Prove generates a Merkle proof for a target generalized index
func (s *SszInfo) Prove(gindex uint64) (*fastssz.Proof, error)
```

## Type System

### SSZ Types

The package now supports [all standard SSZ types](https://github.com/ethereum/consensus-specs/blob/master/ssz/simple-serialize.md#typing) except `ProgressiveList`, `ProgressiveContainer`, `ProgressiveBitlist`, `Union` and `CompatibleUnion`.

### Core Data Structures

#### `SszInfo`

The `SszInfo` structure contains complete structural metadata for an SSZ type:

```go
type SszInfo struct {
   sszType       SszType           // SSZ Type classification
   typ           reflect.Type      // Go reflect.Type
   source        SSZObject         // Original SSZObject reference. Mostly used for reusing SSZ methods like `HashTreeRoot`.
   isVariable    bool              // True if contains variable-size fields

   // Composite types have corresponding metadata. Other fields would be nil except for the current type.
   containerInfo *containerInfo
   listInfo      *listInfo
   vectorInfo    *vectorInfo
   bitlistInfo   *bitlistInfo
   bitvectorInfo *bitvectorInfo
}
```

#### `Path`

The `Path` structure represents navigation paths through SSZ structures. It supports accessing a field by field name, accessing an element by index (list/vector type), and finding the length of homogenous collection types. The `ParsePath` function parses a raw string into a `Path` instance, which is commonly used in other APIs like `CalculateOffsetAndLength` and `GetGeneralizedIndexFromPath`.

```go
type Path struct {
   Length   bool           // Flag for length queries (e.g., len(.field))
   Elements []PathElement  // Sequence of field accesses and indices
}

type PathElement struct {
   Name  string  // Field name
   Index *uint64 // list/vector index (nil if not an index access)
}
```

## Implementation Details

### Type Analysis (`analyzer.go`)

The `AnalyzeObject` function performs recursive type introspection using Go reflection:

1. **Type Inspection** - Examines Go `reflect.Value` to determine SSZ type
   - Basic types (`uint8`, `uint16`, `uint32`, `uint64`, `bool`): `SSZType` constants
   - Slices: Determined from struct tags. (`ssz-size` for vectors, `ssz-max` for lists) There is a related [write-up](https://hackmd.io/@junsong/H101DKnwxl) regarding struct tags.
   - Structs: Analyzed as Containers with field ordering from JSON tags
   - Pointers: Dereferenced automatically

2. **Variable-Length Population** - Determines actual sizes at runtime
   - For Lists: Iterates elements, caches sizes for variable-element lists
   - For Containers: Recursively populates variable fields, adjusts offsets
   - For Bitlists: Decodes bit length from bitvector

3. **Offset Calculation** - Computes byte positions within serialized data
   - Fixed-size fields: Offset = sum of preceding field sizes
   - Variable-size fields: Offset stored as 4-byte pointer entries

### Path Parsing (`path.go`)

The `ParsePath` function parses path strings with the following rules:

- **Dot notation**: `.field1.field2` for field access
- **Array indexing**: `[0]`, `[42]` for element access
- **Length queries**: `len(.field)` for list/vector lengths
- **Character set**: Only `[A-Za-z0-9._\[\]\(\)]` allowed

Example:
```go
path, _ := ParsePath(".nested.array_field[5].inner_field")
// Returns: Path{
//   Elements: [
//     PathElement{Name: "nested"},
//     PathElement{Name: "array_field", Index: <Pointer to uint64(5)>},
//     PathElement{Name: "inner_field"}
//   ]
// }
```

### Generalized Index Calculation (`generalized_index.go`)

The generalized index is a tree position identifier. This package follows the [Ethereum consensus-specs](https://github.com/ethereum/consensus-specs/blob/master/ssz/merkle-proofs.md#generalized-merkle-tree-index) to calculate the generalized index.

### Merkle Proof Generation (`merkle_proof.go`, `proof_collector.go`)

The `Prove` method generates Merkle proofs using a single-sweep merkleization algorithm:

#### Algorithm Overview

1. **Registration Phase** (`addTarget`)
   - Marks the target gindex
   - Marks every required sibling node by walking from the target leaf to the root (gindex=1).

2. **Merkleization Phase** (`merkleize`)
   - Traverses data structure recursively
   - Builds Merkle tree from leaves to root
   - Collects hashes at registered gindices during traversal

3. **Proof Conversion Phase** (`toProof`)
   - Converts collected hashes to `fastssz.Proof`
   - Stores leaf value at proof index
   - Orders sibling hashes from leaf upward

#### Type-Specific Merkleization

   - **Basic Types**: Merkleize as a single 32-byte chunk containing the SSZ-encoded value, little-endian and zero-padded.
   - **Containers**: Merkleize by computing the 32-byte root of each field, then merkleizing the field roots.
   - **Vectors**: Merkleize fixed-size collections by chunking elements (basic types are packed; composite types use their 32-byte roots).
   - **Lists**: Merkleize variable-size collections by chunking elements up to the type’s maximum, then mix in the actual element count.
   - **Bitvectors**: Merkleize packed bits in 32-byte, LSB-first chunks, using the type’s chunk count.
   - **Bitlists**: Merkleize packed bits in 32-byte, LSB-first chunks (excluding the termination bit), using the type’s chunk count, then mix in the bit length.

## Dependencies

### Internal Packages
- `encoding/ssz` - Core SSZ utilities (`Depth()`, `PackByChunk()`, `MixInLength()`)
- `container/trie` - Zero hash caching (`trie.ZeroHashes`)
- `crypto/hash/htr` - Vectorized hashing (`htr.VectorizedSha256()`)

### External Libraries
- `github.com/prysmaticlabs/fastssz` - SSZ serialization framework (`fastssz.Proof`)
- `github.com/prysmaticlabs/go-bitfield` - Bitlist/Bitvector handling
