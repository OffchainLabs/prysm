# SSZ Query Package

The `encoding/ssz/query` package provides a system for analyzing and querying SSZ ([Simple Serialize](https://github.com/ethereum/consensus-specs/blob/master/ssz/simple-serialize.md)) data structures, as well as generating Merkle proofs from them. It enables runtime analysis of SSZ-serialized Go objects with reflection, path-based queries through nested structures, generalized index calculation, and Merkle proof generation.

This package is designed to be generic. It operates on arbitrary SSZ-serialized Go values at runtime, so the same query/proof machinery applies equally to any SSZ type, including the BeaconState/BeaconBlock.

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

The package now supports [all standard SSZ types](https://github.com/ethereum/consensus-specs/blob/master/ssz/simple-serialize.md#typing) except `ProgressiveList`, `ProgressiveContainer`, `ProgressiveBitlist`, `Union`, and `CompatibleUnion`.

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
   - Slices: Determined from struct tags (`ssz-size` for vectors, `ssz-max` for lists). There is a related [write-up](https://hackmd.io/@junsong/H101DKnwxl) regarding struct tags.
   - Structs: Analyzed as Containers with field ordering from JSON tags
   - Pointers: Dereferenced automatically

2. **Variable-Length Population** - Determines actual sizes at runtime
   - For lists: Iterates elements, caches sizes for variable-element lists
   - For containers: Recursively populates variable fields, adjusts offsets
   - For bitlists: Decodes bit length from bitvector

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

**Key Terms:**

- **Target gindex** (generalized index): The position of the SSZ element you want to prove, expressed as a generalized Merkle tree index. Stored in `Proof.Index`.
  - Note: The generalized index for root is 1.
- **Registered gindices**: The set of tree positions whose node hashes must be captured during merkleization in order to later assemble the proof.
- **Sibling node**: The node that shares the same parent as another node.
- **Leaf value**: The 32-byte hash of the target node (the node being proven). Stored in `Proof.Leaf`.

**Phases:**

1. **Registration Phase** (`addTarget`)
> Goal: determine exactly which sibling hashes are needed for the proof.

   - Record the target gindex as the proof target.
   - Starting from the target node, walk the Merkle tree from the leaf (target gindex) to the root (gindex = 1).
   - At each step:
     - Compute and register the sibling gindex (`i XOR 1`) as “must collect”.
     - Move to the parent (`i = i/2`).
   - This produces the full set of registered gindices (the sibling nodes on the target-to-root path).

2. **Merkleization Phase** (`merkleize`)
> Goal: recursively merkleize the tree and capture the needed hashes.

   - Recursively traverse the SSZ structure and compute Merkle tree node hashes from leaves to root.
   - Whenever the traversal computes a node whose gindex is in registered gindices, store that node’s hash for later proof construction.

3. **Proof Assembly Phase** (`toProof`)
> Goal: create the final `fastssz.Proof` object in the correct format and order.

```go
// Proof represents a merkle proof against a general index.
type Proof struct {
	Index  int
	Leaf   []byte
	Hashes [][]byte
}
```

   - Set `Proof.Index` to the target gindex.
   - Set `Proof.Leaf` to the 32-byte hash of the target node.
   - Build `Proof.Hashes` by walking from the target node up to (but not including) the root:
     - At node `i`, append the stored hash for the sibling (`i XOR 1`).
     - Move to the parent (`i = i/2`).
   - The resulting `Proof.Hashes` is ordered from the target level upward, containing one sibling hash per tree level on the path to the root.
