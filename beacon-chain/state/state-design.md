# Beacon Chain State Design Document

## Package Structure

### `/beacon-chain/state`

This is the top-level state package that defines interfaces only. This package provides a clean API boundary for state access without implementation details. The package follows the interface segregation principle, breaking down access patterns into granular read-only and write-only interfaces (e.g., `ReadOnlyValidators`, `WriteOnlyValidators`, `ReadOnlyBalances`, `WriteOnlyBalances`).

### `/beacon-chain/state/state-native`

This package contains the actual `BeaconState` struct and all concrete implementations of the state interfaces. It also contains thread-safe getters and setters.

### `/beacon-chain/state/fieldtrie`

A dedicated package for field-level merkle trie functionality. The main component of the package is the `FieldTrie` struct which represents a particular field's merkle trie. The package contains several field trie operations such as recomputing tries, copying and transferring them.

### `/beacon-chain/state/stateutil`

Utility package whose main components are various state merkleization functions. Other things contained in this package include field trie helpers, the implementation of shared references, and validator tracking.

## Beacon State Architecture

### `BeaconState` Structure

The `BeaconState` is a single structure that supports all consensus versions. The value of the `version` field determines the active version of the state. It is used extensively throughout the codebase to determine which code path will be executed.

```go
type BeaconState struct {
    version int
    id      uint64 // Used for tracking states in multi-value slices

    // Common fields (all versions)
    genesisTime           uint64
    genesisValidatorsRoot [32]byte
    slot                  primitives.Slot
    // ...

    // Phase0+ fields
    eth1Data                  *ethpb.Eth1Data
    eth1DataVotes             []*ethpb.Eth1Data
    eth1DepositIndex          uint64
    slashings                 []uint64
    previousEpochAttestations []*ethpb.PendingAttestation
    currentEpochAttestations  []*ethpb.PendingAttestation
    // ...

    // Altair+ fields
    previousEpochParticipation []byte
    currentEpochParticipation  []byte
    currentSyncCommittee       *ethpb.SyncCommittee
    nextSyncCommittee          *ethpb.SyncCommittee
    // ...

    // Fields for other forks...

    // Internal state management
    lock                  sync.RWMutex
    dirtyFields           map[types.FieldIndex]bool
    dirtyIndices          map[types.FieldIndex][]uint64
    stateFieldLeaves      map[types.FieldIndex]*fieldtrie.FieldTrie
    rebuildTrie           map[types.FieldIndex]bool
    valMapHandler         *stateutil.ValidatorMapHandler
    merkleLayers          [][][]byte
    sharedFieldReferences map[types.FieldIndex]*stateutil.Reference
}
```

### Multi-Value Slices

Several large array fields of the state are implemented as multi-value slices. This is a specialized data structure that enables efficient sharing and modification of slices between states. A multi-value slice is preferred over a regular slice in scenarios where only a small fraction of the array is updated at a time. In such cases, using a multi-value slice results in fewer memory allocations because many values of the slice will be shared between states, whereas with a regular slice changing even a single item results in copying the full slice. Examples include `MultiValueBlockRoots` and `MultiValueBalances`.

**Example**
```go
// Create new multi-value slice
mvBalances := NewMultiValueBalances([]uint64{32000000000, 32000000000, ...})

// Share across states
state1.balancesMultiValue = mvBalances
state2 := state1.Copy()  // state2 shares the same mvBalances

// Modify in state2
state2.UpdateBalancesAtIndex(0, 31000000000)  // This doesn't create a new multi-value slice.

state1.BalanceAtIndex(0) // this returns 32000000000
state2.BalanceAtIndex(0) // this returns 31000000000
```

### Getters/Setters

All beacon state getters and setters follow a consistent pattern. All exported methods are protected from concurrent modification using read locks (for getters) or write locks (for setters).

**Getters**

Values are never returned directly from getters. A copy of the value is returned instead.

```go
func (b *BeaconState) Fork() *ethpb.Fork {
    if b.fork == nil {
        return nil
    }

    b.lock.RLock()
    defer b.lock.RUnlock()

    return b.forkVal()
}

func (b *BeaconState) forkVal() *ethpb.Fork {
    if b.fork == nil {
        return nil
    }

    prevVersion := make([]byte, len(b.fork.PreviousVersion))
    copy(prevVersion, b.fork.PreviousVersion)
    currVersion := make([]byte, len(b.fork.CurrentVersion))
    copy(currVersion, b.fork.CurrentVersion)
    return &ethpb.Fork{
        PreviousVersion: prevVersion,
        CurrentVersion:  currVersion,
        Epoch:           b.fork.Epoch,
    }
}
```

```go
func (b *BeaconState) CurrentEpochAttestations() ([]*ethpb.PendingAttestation, error) {
    if b.version != version.Phase0 {
        return nil, errNotSupported("CurrentEpochAttestations", b.version)
    }

    if b.currentEpochAttestations == nil {
        return nil, nil
    }

    b.lock.RLock()
    defer b.lock.RUnlock()

    return b.currentEpochAttestationsVal(), nil
}

func (b *BeaconState) currentEpochAttestationsVal() []*ethpb.PendingAttestation {
    if b.currentEpochAttestations == nil {
        return nil
    }

    res := make([]*ethpb.PendingAttestation, len(b.currentEpochAttestations))
    for i := range res {
        res[i] = b.currentEpochAttestations[i].Copy()
    }
    return res
}
```

In the case of fields backed by multi-value slices, the appropriate methods of the multi-value slice are invoked.

```go
func (b *BeaconState) StateRoots() [][]byte {
    b.lock.RLock()
    defer b.lock.RUnlock()

    roots := b.stateRootsVal()
    if roots == nil {
        return nil
    }
    return roots.Slice()
}

func (b *BeaconState) stateRootsVal() customtypes.StateRoots {
    if b.stateRootsMultiValue == nil {
        return nil
    }
    return b.stateRootsMultiValue.Value(b)
}

func (b *BeaconState) StateRootAtIndex(idx uint64) ([]byte, error) {
    b.lock.RLock()
    defer b.lock.RUnlock()

    if b.stateRootsMultiValue == nil {
        return nil, nil
    }
    r, err := b.stateRootsMultiValue.At(b, idx)
    if err != nil {
        return nil, err
    }
    return r[:], nil
}
```

**Setters**

Whenever a beacon state field is set, it is marked as dirty. This is needed for hash tree root computation so that the cached merkle branch with the old value of the modified field is recomputed using the new value.

```go
func (b *BeaconState) SetSlot(val primitives.Slot) error {
    b.lock.Lock()
    defer b.lock.Unlock()

    b.slot = val
    b.markFieldAsDirty(types.Slot)
    return nil
}
```

Several fields of the state are shared between states through a `Reference` mechanism. These references are stored in `b.sharedFieldReferences`. Whenever a state is copied, the reference counter for each of these fields is incremented. When a new value for any of these fields is set, the counter for the existing reference is decremented and a new reference is created for that field.

```go
type Reference struct {
    refs uint // Reference counter
    lock sync.RWMutex
}
```

```go
func (b *BeaconState) SetCurrentParticipationBits(val []byte) error {
    b.lock.Lock()
    defer b.lock.Unlock()

    if b.version == version.Phase0 {
        return errNotSupported("SetCurrentParticipationBits", b.version)
    }

    b.sharedFieldReferences[types.CurrentEpochParticipationBits].MinusRef()
    b.sharedFieldReferences[types.CurrentEpochParticipationBits] = stateutil.NewRef(1)

    b.currentEpochParticipation = val
    b.markFieldAsDirty(types.CurrentEpochParticipationBits)
    return nil
}
```

Updating a single value of an array field requires updating `b.dirtyIndices` to ensure the field trie for that particular field is properly recomputed.

```go
func (b *BeaconState) UpdateBalancesAtIndex(idx primitives.ValidatorIndex, val uint64) error {
    if err := b.balancesMultiValue.UpdateAt(b, uint64(idx), val); err != nil {
        return errors.Wrap(err, "could not update balances")
    }

    b.lock.Lock()
    defer b.lock.Unlock()

    b.markFieldAsDirty(types.Balances)
    b.addDirtyIndices(types.Balances, []uint64{uint64(idx)})
    return nil
}
```

As is the case with getters, for fields backed by multi-value slices the appropriate methods of the multi-value slice are invoked when updating field values. The multi-value slice keeps track of states internally, which means the `Reference` construct is unnecessary.

```go
func (b *BeaconState) SetStateRoots(val [][]byte) error {
    b.lock.Lock()
    defer b.lock.Unlock()

    if b.stateRootsMultiValue != nil {
        b.stateRootsMultiValue.Detach(b)
    }
    b.stateRootsMultiValue = NewMultiValueStateRoots(val)

    b.markFieldAsDirty(types.StateRoots)
    b.rebuildTrie[types.StateRoots] = true
    return nil
}

func (b *BeaconState) UpdateStateRootAtIndex(idx uint64, stateRoot [32]byte) error {
    if err := b.stateRootsMultiValue.UpdateAt(b, idx, stateRoot); err != nil {
        return errors.Wrap(err, "could not update state roots")
    }

    b.lock.Lock()
    defer b.lock.Unlock()

    b.markFieldAsDirty(types.StateRoots)
    b.addDirtyIndices(types.StateRoots, []uint64{idx})
    return nil
}
```

### Read-Only Validator

There are two ways in which validators can be accessed through getters. One approach is to use the methods `Validators` and `ValidatorAtIndex`, which return a copy of, respectively, the whole validator set or a particular validator. The other approach is to use the `ReadOnlyValidator` construct via the `ValidatorsReadOnly` and `ValidatorAtIndexReadOnly` methods. The `ReadOnlyValidator` structure is a wrapper around the protobuf validator. The advantage of using the read-only version is that no copy of the underlying validator is made, which helps with performance, especially when accessing a large number of validators (e.g. looping through the whole validator registry). Because the read-only wrapper exposes only getters, each of which returns a copy of the validator’s fields, it prevents accidental mutation of the underlying validator.

```go
type ReadOnlyValidator struct {
    validator *ethpb.Validator
}

// Only getter methods, no setters
func (v *ReadOnlyValidator) PublicKey() [48]byte
func (v *ReadOnlyValidator) EffectiveBalance() uint64
func (v *ReadOnlyValidator) Slashed() bool
// ... etc
```

**Usage**
```go
// Returns ReadOnlyValidator
validator, err := state.ValidatorAtIndexReadOnly(idx)

// Can read but not modify
pubkey := validator.PublicKey()
balance := validator.EffectiveBalance()

// To modify, must get mutable copy
validatorCopy := validator.Copy()  // Returns *ethpb.Validator
validatorCopy.EffectiveBalance = newBalance
state.UpdateValidatorAtIndex(idx, validatorCopy)
```

## Field Trie System

For a few large arrays, such as block roots or the validator registry, hashing the state field at every slot would be very expensive. To avoid such re-hashing, the underlying merkle trie of the field is maintained and only the branch(es) corresponding to the changed index(es) are recomputed, instead of the whole trie.

Each state version has a list of active fields defined (e.g. `phase0Fields`, `altairFields`), which serves as the basis for field trie creation.

```go
type FieldTrie struct {
    *sync.RWMutex
    reference     *stateutil.Reference     // The number of states this field trie is shared between
    fieldLayers   [][]*[32]byte            // Merkle trie layers
    field         types.FieldIndex         // Which field this field trie represents
    dataType      types.DataType           // Type of field's array
    length        uint64                   // Maximum number of elements
    numOfElems    int                      // Number of elements
    isTransferred bool                     // Whether trie was transferred
}
```

The `DataType` enum indicates the type of the field's array. The possible values are:
- `BasicArray`: Fixed-size arrays (e.g. `blockRoots`)
- `CompositeArray`: Variable-size arrays (e.g. `validators`)
- `CompressedArray`: Variable-size arrays that pack multiple elements per trie leaf (e.g. `balances` with 4 elements per leaf)

### Recomputing a Trie

To avoid recomputing the state root every time any value of the state changes, branches of field tries are not recomputed until the state root is actually needed. When it is necessary to recompute a trie, the `RecomputeTrie` function rebuilds the affected branches in the trie according to the provided changed indices. The changed indices of each field are tracked in the `dirtyIndices` field of the beacon state, which is a `map[types.FieldIndex][]uint64`. The recomputation algorithms for fixed-size and variable-size fields are different.

### Transferring a Trie

When it is expected that an older state won't need its trie for recomputation, its trie layers can be transferred directly to a new trie instead of copying them:

```go
func (f *FieldTrie) TransferTrie() *FieldTrie {
    f.isTransferred = true
    nTrie := &FieldTrie{
        fieldLayers: f.fieldLayers,  // Direct transfer, no copy
        field:       f.field,
        dataType:    f.dataType,
        reference:   stateutil.NewRef(1),
        // ...
    }
    f.fieldLayers = nil  // Zero out original
    return nTrie
}
```

The downside of this approach is that if it becomes necessary to access the older state's trie later on, the whole trie would have to be recreated since it is empty now. This is especially costly for the validator registry and that is why it is always copied instead.

```go
if fTrie.FieldReference().Refs() > 1 {
    var newTrie *fieldtrie.FieldTrie
    // We choose to only copy the validator
    // trie as it is pretty expensive to regenerate.
    if index == types.Validators {
        newTrie = fTrie.CopyTrie()
    } else {
        newTrie = fTrie.TransferTrie()
    }
    fTrie.FieldReference().MinusRef()
    b.stateFieldLeaves[index] = newTrie
    fTrie = newTrie
}
```

## Hash Tree Root Computation

The `BeaconState` structure is not a protobuf object, so there are no SSZ-generated methods for its fields. For each state field, there exists a custom method that returns its root. One advantage of doing it this way is the possibility to have the most efficient implementation possible. As an example, when computing the root of the validator registry, validators are hashed in parallel and vectorized sha256 computation is utilized to speed up the computation.

```go
func OptimizedValidatorRoots(validators []*ethpb.Validator) ([][32]byte, error) {
    // Exit early if no validators are provided.
    if len(validators) == 0 {
        return [][32]byte{}, nil
    }
    wg := sync.WaitGroup{}
    n := runtime.GOMAXPROCS(0)
    rootsSize := len(validators) * validatorFieldRoots
    groupSize := len(validators) / n
    roots := make([][32]byte, rootsSize)
    wg.Add(n - 1)
    for j := 0; j < n-1; j++ {
        go hashValidatorHelper(validators, roots, j, groupSize, &wg)
    }
    for i := (n - 1) * groupSize; i < len(validators); i++ {
        fRoots, err := ValidatorFieldRoots(validators[i])
        if err != nil {
            return [][32]byte{}, errors.Wrap(err, "could not compute validators merkleization")
        }
        for k, root := range fRoots {
            roots[i*validatorFieldRoots+k] = root
        }
    }
    wg.Wait()

    // A validator's tree can represented with a depth of 3. As log2(8) = 3
    // Using this property we can lay out all the individual fields of a
    // validator and hash them in single level using our vectorized routine.
    for range validatorTreeDepth {
        // Overwrite input lists as we are hashing by level
        // and only need the highest level to proceed.
        roots = htr.VectorizedSha256(roots)
    }
    return roots, nil
}
```

Calculating the full state root is very expensive; therefore, it is done in a lazy fashion. Previously generated merkle layers are cached in the state and only merkle trie branches corresponding to dirty indices are regenerated before returning the final root.

```go
func (b *BeaconState) HashTreeRoot(ctx context.Context) ([32]byte, error) {
    ctx, span := trace.StartSpan(ctx, "beaconState.HashTreeRoot")
    defer span.End()

    b.lock.Lock()
    defer b.lock.Unlock()
    
    // When len(b.merkleLayers) > 0, this function returns immediately
    if err := b.initializeMerkleLayers(ctx); err != nil {
        return [32]byte{}, err
    }
    // Dirty fields are tracked in the beacon state through b.dirtyFields
    if err := b.recomputeDirtyFields(ctx); err != nil {
        return [32]byte{}, err
    }
    return bytesutil.ToBytes32(b.merkleLayers[len(b.merkleLayers)-1][0]), nil
}
```