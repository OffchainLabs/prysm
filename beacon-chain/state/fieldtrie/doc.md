# Field Trie: Design and Implementation

## 1. Computing the Beacon State Root

The beacon state is composed of dozens of fields (block roots, state roots, validators,
balances, randao mixes, etc.). To produce the state root, we must compute the hash-tree-root
(HTR) of every field and then combine them into a single Merkle root that represents the
entire state.

## 2. Per-Field Roots

Each field is a collection of elements (a fixed-size or variable-length list). Before they
can be folded into the top-level state root, every field must first be reduced to a single
32-byte root. This is done by hashing all its elements together according to the SSZ
specification.

## 3. Why Merkle Tries

Recomputing the root of a field from scratch every time would require hashing all elements,
which is expensive for large fields (e.g., the validator registry, which can exceed one
million entries). To avoid this, the SSZ spec defines the hash-tree-root as the root of a
binary Merkle trie built over the field's elements. By maintaining the full trie in memory,
we can perform incremental updates rather than full rebuilds.

## 4. Incremental Recomputation

When only a few elements in a field change between state transitions, we do not need to
recompute the entire trie. We only need to recompute the branches from each changed leaf up
to the root. For `k` changed leaves in a trie of depth `d`, this costs `O(k * d)` hash
operations instead of `O(n)` for a full rebuild. In practice, between two consecutive slots,
only a small fraction of leaves change in most fields, so this is a significant saving.


Consider a trie with 8 leaves. Only leaf F changes (to F'):

```
            R                                R'
          /   \                            /   \
        H1     H2                        H1     H2'
       / \    / \                       / \     / \
     H3  H4  H5  H6                   H3  H4  H5'  H6
     /\  /\  /\  /\                   /\  /\  /\   /\
    A B  C D E F G H                 A B  C D E F' G H

     Full trie before                After updating leaf F
```

Only the nodes on the path from the changed leaf to the root need recomputing (marked with
`'`). Everything else is reused as-is:

```
  Recomputation path:

    1. Hash the new leaf F'
    2. H5' = Hash(E, F')          ← sibling E is reused
    3. H2' = Hash(H5', H6)        ← sibling H6 is reused
    4. R'  = Hash(H1, H2')        ← sibling H1 is reused

  Cost: 4 hashes instead of 15 (full rebuild).
  In general: O(log n) instead of O(n).
```

## 5. Flat Buffer Storage

Trie nodes are stored in a single contiguous flat buffer (`[][32]byte`) rather than as a
pointer-based tree structure. Each level of the trie is packed contiguously in the buffer,
from leaves (level 0) up to the root (level `depth`).

An offsets table (`[]uint64`) maps each level to its starting index in the buffer:
`offsets[level]` is the start of that level's nodes, and `offsets[level+1] - offsets[level]`
gives the number of nodes at that level. The last entry `offsets[depth+1]` equals the total
buffer length.

For example, a trie with 4 leaves and depth 2 is laid out as:

```
nodes   = [A, B, C, D, H(A,B), H(C,D), H(H(A,B),H(C,D))]
offsets = [0, 4, 6, 7]

Level 0 (leaves):   nodes[0..4)   = A, B, C, D
Level 1:            nodes[4..6)   = H(A,B), H(C,D)
Level 2 (root):     nodes[6..7)   = H(H(A,B), H(C,D))
```

This layout is cache-friendly for both full rebuilds (sequential access through the buffer)
and branch recomputation (known offsets for each level).

When the buffer needs to grow (e.g., new validators are added), it is reallocated with 10%
headroom to amortize repeated growth.

## 6. Copy-on-Write: Base + Overlay + Promotion

Multiple beacon state instances often share the same field data (e.g., forked states during
attestation processing). Copying the entire flat buffer for each state copy would be
wasteful. Instead, the implementation uses a **copy-on-write overlay** system.

### Owned Mode

A trie in **owned mode** holds its own flat buffer (`nodesData`). This is the default mode
when a trie is first built. It can serve as a read-only base for overlays.

### Overlay Mode

When a shared trie needs to be mutated, a **fork** creates a new trie in **overlay mode**.
An overlay does not copy the flat buffer. Instead, it holds:

- A pointer to the immutable **base** trie (the original owned trie).
- A sparse **overrides** structure: one `map[uint64][32]byte` per trie level, storing only
  the nodes that differ from the base.

Reading a node in overlay mode first checks the overrides map for that level; if absent, it
falls back to the base buffer. Writing only touches the overrides maps.

This means copying a trie is cheap: the overlay starts empty, and only the nodes on branches
that are actually modified get stored. Multiple overlays can share the same base.

### Reference Counting

Two reference counts track sharing:

- `ref`: how many `FieldTrie` handles point to the same trie (incremented on `CopyTrie`,
  decremented by GC cleanup).
- `dataRef`: how many overlays are using this trie's buffer as their base. The base buffer
  stays alive as long as any overlay still references it.

A trie is considered **shared** when `ref > 1 || dataRef > 0`.

When a mutation is requested on a shared trie, `fork()` creates a fresh trie. If the source
is in owned mode, the fork becomes an overlay pointing at the source. If the source is
already an overlay, the fork deep-copies the overrides maps while sharing the same base.

### Promotion

As an overlay accumulates changes, the per-lookup map overhead grows. When the number of
dirty leaf overrides exceeds a threshold (default: 20,000, or a configurable fraction of
total leaves), the overlay is **promoted** back to owned mode: a fresh flat buffer is
allocated, the base data is copied in, all overrides are applied on top, the full trie is
recomputed from leaves, and the base reference is released.

This ensures that overlays remain efficient for the common case (few changes per slot) while
gracefully degrading to a full rebuild when accumulated drift becomes too large.

## 7. Usage Example

The following Go code illustrates the lifecycle of a `FieldTrie` across creation, copying,
root computation, recomputation, and garbage collection, with the state of the two reference
counters (`Ref` and `DataRef`) annotated at every step.

- `Ref` tracks how many `FieldTrie` handles share the same underlying trie.
- `DataRef` tracks how many overlay tries are using this trie's flat buffer as their base.

A trie is considered **shared** (and therefore triggers a fork on mutation) when
`Ref > 1 || DataRef > 0`.

```go
package main

import (
	"fmt"
	"runtime"

	fieldtrie "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/fieldtrie"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/types"
)

func main() {
	// ---------------------------------------------------------------
	// Step 1: Create a new field trie (owned mode).
	// ---------------------------------------------------------------
	elements := make([][]byte, 8)
	for i := range elements {
		elements[i] = []byte{byte(i)}
	}

	trieA, err := fieldtrie.NewFieldTrie(
		types.BlockRoots,    // field index
		types.BasicArray,    // data type
		elements,            // initial elements
		8,                   // max capacity (determines trie depth)
		0,                   // promotionThreshold (0 = use default)
	)
	if err != nil {
		panic(err)
	}

	// trieA is in owned mode with its own flat buffer.
	//   trieA.Ref     = 1   (one handle: trieA)
	//   trieA.DataRef = 0   (no overlay is using trieA as a base)

	// ---------------------------------------------------------------
	// Step 2: Compute the root of the trie (read-only).
	// ---------------------------------------------------------------
	rootA, err := trieA.TrieRoot()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Root A: %x\n", rootA)

	// TrieRoot is a read-only operation — reference counts are unchanged.
	//   trieA.Ref     = 1
	//   trieA.DataRef = 0

	// ---------------------------------------------------------------
	// Step 3: Copy the trie (lightweight, reference-shared copy).
	// ---------------------------------------------------------------
	trieB := trieA.CopyTrie()

	// CopyTrie increments the shared ref counter.
	// trieA and trieB point to the SAME underlying data.
	//   trieA.Ref     (== trieB.Ref)     = 2   (two handles share the trie)
	//   trieA.DataRef (== trieB.DataRef) = 0

	// ---------------------------------------------------------------
	// Step 4: Recompute trieB with changed elements.
	// ---------------------------------------------------------------
	// Because Ref == 2 (shared), RecomputeTrie will fork before mutating.
	// The fork creates a new independent trie (trieC) in overlay mode,
	// pointing to trieA's flat buffer as its immutable base.
	changedIndices := []uint64{0, 3}
	newElements := make([][]byte, 8)
	copy(newElements, elements)
	newElements[0] = []byte{0xAA}
	newElements[3] = []byte{0xBB}

	trieC, rootC, err := trieB.RecomputeTrie(changedIndices, newElements)
	if err != nil {
		panic(err)
	}

	// trieB is now stale — the caller MUST use trieC instead.
	// Internally, fork() did the following:
	//   - Created trieC with fresh Ref=1, DataRef=0.
	//   - Incremented trieA.DataRef (trieC uses trieA's buffer as base).
	//   - trieA/trieB still share Ref=2 (unchanged by the fork).
	//
	// After fork + recompute:
	//   trieA.Ref     = 2   (trieA and trieB still share)
	//   trieA.DataRef = 1   (trieC's overlay references trieA's buffer)
	//   trieC.Ref     = 1   (only trieC holds the forked trie)
	//   trieC.DataRef = 0   (no overlay is based on trieC)

	fmt.Printf("Root C: %x\n", rootC)

	// ---------------------------------------------------------------
	// Step 5: GC trieB — one of the two original shared handles.
	// ---------------------------------------------------------------
	trieB = nil
	runtime.GC()

	// The GC cleanup callback (cleanupRef) decrements the shared Ref.
	//   trieA.Ref     = 1   (only trieA remains)
	//   trieA.DataRef = 1   (trieC's overlay still references trieA's buffer)
	//   trieC.Ref     = 1
	//   trieC.DataRef = 0

	// ---------------------------------------------------------------
	// Step 6: GC trieC — the overlay is released.
	// ---------------------------------------------------------------
	_ = rootC
	trieC = nil
	runtime.GC()

	// The GC cleanup callbacks fire:
	//   - cleanupRef on trieC.Ref   → trieC.Ref decremented (now 0)
	//   - cleanupRef on trieA.DataRef (via dataRefCleanup) → trieA.DataRef decremented
	//
	//   trieA.Ref     = 1   (trieA is still alive)
	//   trieA.DataRef = 0   (no more overlays reference trieA's buffer)

	// trieA is no longer shared (Ref == 1 && DataRef == 0),
	// so future RecomputeTrie calls on trieA will mutate in place
	// without forking.

	// ---------------------------------------------------------------
	// Step 7: Recompute trieA in place (not shared anymore).
	// ---------------------------------------------------------------
	newElements[1] = []byte{0xCC}
	trieA, rootA, err = trieA.RecomputeTrie([]uint64{1}, newElements)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Root A (updated): %x\n", rootA)

	// Because trieA was not shared, no fork happened — it was mutated
	// in place. Reference counts remain:
	//   trieA.Ref     = 1
	//   trieA.DataRef = 0

	// ---------------------------------------------------------------
	// Step 8: GC trieA — final cleanup.
	// ---------------------------------------------------------------
	trieA = nil
	runtime.GC()

	// The GC cleanup callback decrements the Ref counter.
	//   Ref     = 0   (no more handles, trie data can be freed)
	//   DataRef = 0
}
```
