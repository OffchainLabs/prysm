package fieldtrie

import (
	"encoding/binary"
	"fmt"
	"reflect"

	customtypes "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/custom-types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	multi_value_slice "github.com/OffchainLabs/prysm/v7/container/multi-value-slice"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	pmath "github.com/OffchainLabs/prysm/v7/math"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// trieMode identifies whether a field trie is in owned or overlay mode.
type trieMode string

const (
	trieModeOwned   trieMode = "owned"
	trieModeOverlay trieMode = "overlay"
)

// buildTrie converts elements to leaf hashes and builds a Merkle trie, returning all
// nodes packed into a single contiguous buffer.
//
// It returns:
//   - nodes: the flat buffer, with leaves first, then each upper level in order.
//   - offsets: offsets[i] is the start index of level i; offsets[depth+1] = len(nodes).
//
// The last item in the offsets array, added for convenience, is the total length of the nodes array.
//
// Example 1: 4 leaves, length=4 (depth = 2):
//
// Level 2 (root):    H(H(A,B), H(C,D))
// Level 1:           H(A,B)   H(C,D)
// Level 0 (leaves):  A  B  C  D
//
// nodes   = [A, B, C, D, H(A,B), H(C,D), H(H(A,B),H(C,D))]
// offsets = [0, 4, 6, 7]
//
// Example 2: 5 leaves, length=10 (depth = 4):
//
// Level 4 (root):    H(H(H(H(A,B),H(C,D)),H(H(E,Z0),Z1)), Z3)
// Level 3:           H(H(H(A,B),H(C,D)), H(H(E,Z0),Z1))
// Level 2:           H(H(A,B),H(C,D))    H(H(E,Z0),Z1)
// Level 1:           H(A,B)  H(C,D)  H(E,Z0)
// Level 0 (leaves):  A  B  C  D  E
//
// Zn denotes ZeroHashes[n]
//
// nodes   = [A, B, C, D, E, H(A,B), H(C,D), H(E,Z0), H(H(A,B),H(C,D)), H(H(E,Z0),Z1), H(...), root]
// offsets = [0, 5, 8, 10, 11, 12]
func buildTrie(field types.FieldIndex, elements any, length uint64) ([][32]byte, []uint64, error) {
	if elements == nil {
		return nil, nil, nil
	}

	fieldRoots, err := fieldConverters(field, elements, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("field converters: %w", err)
	}

	depth := uint64(ssz.Depth(length))
	count := uint64(len(fieldRoots))

	if count == 0 {
		nodes := [][32]byte{trie.ZeroHashes[depth]}
		offsets := make([]uint64, depth+2)
		offsets[depth+1] = 1
		return nodes, offsets, nil
	}

	offsets := computeOffsets(depth, count)

	trieNodeCount := offsets[depth+1]
	nodes := make([][32]byte, trieNodeCount)

	// Compute all upper levels of the trie from the leaves up to the root.
	copy(nodes, fieldRoots)
	hashUpFromLeaves(nodes, offsets)

	return nodes, offsets, nil
}

// hashUpFromLeaves computes all upper levels of a flat trie buffer from level 0.
// nodes[offsets[0]:offsets[1]] must be pre-filled with leaf data.
// Odd-length levels are handled by hashing the last element with ZeroHashes[level].
func hashUpFromLeaves(nodes [][32]byte, offsets []uint64) {
	depth := uint64(len(offsets) - 2)
	hasher := hash.CustomSHA256Hasher()
	var combined [64]byte

	for level := range depth {
		levelSize := offsets[level+1] - offsets[level]
		src := nodes[offsets[level]:offsets[level+1]]
		nextStart := offsets[level+1]

		if levelSize == 1 {
			copy(combined[:32], src[0][:])
			copy(combined[32:], trie.ZeroHashes[level][:])
			nodes[nextStart] = hasher(combined[:])
			continue
		}

		if levelSize%2 == 0 {
			result := htr.VectorizedSha256(src)
			copy(nodes[nextStart:], result)
			continue
		}

		evenPart := levelSize - 1
		result := htr.VectorizedSha256(src[:evenPart])
		copy(nodes[nextStart:], result)
		copy(combined[:32], src[levelSize-1][:])
		copy(combined[32:], trie.ZeroHashes[level][:])
		nodes[nextStart+uint64(len(result))] = hasher(combined[:])
	}
}

// computeOffsets builds the offset table.
// The last entry is the total number of nodes in the trie (added for convenience).
func computeOffsets(depth uint64, leafCount uint64) []uint64 {
	offsets := make([]uint64, depth+2)
	total, size := uint64(0), leafCount
	for i := range depth + 1 {
		offsets[i] = total
		total += size

		if size > 1 {
			size = (size + 1) / 2
		}
	}

	offsets[depth+1] = total

	return offsets
}

func (f *FieldTrie) validateIndices(indices []uint64) error {
	length := f.length

	if f.dataType == types.CompressedArray {
		comLength, err := f.field.ElemsInChunk()
		if err != nil {
			return fmt.Errorf("elem in chunk: %w", err)
		}

		length *= comLength
	}

	for _, index := range indices {
		if index >= length {
			return fmt.Errorf("invalid index for field %s: %d >= length %d", f.field.String(), index, length)
		}
	}

	return nil
}

func validateElements(field types.FieldIndex, fieldInfo types.DataType, elements any, length uint64) error {
	if elements == nil {
		return nil
	}

	if fieldInfo == types.CompressedArray {
		elemsInChunk, err := field.ElemsInChunk()
		if err != nil {
			return fmt.Errorf("elem in chunk: %w", err)
		}

		length *= elemsInChunk
	}

	const message = "elements length is larger than expected for field %s: %d > %d"
	if val, ok := elements.(sliceAccessor); ok {
		totalLen := uint64(val.Len(val.State()))
		if totalLen <= length {
			return nil
		}

		return fmt.Errorf(message, field.String(), totalLen, length)
	}

	val := reflect.Indirect(reflect.ValueOf(elements))
	totalLen := uint64(val.Len())
	if totalLen <= length {
		return nil
	}

	return fmt.Errorf(message, field.String(), totalLen, length)
}

// fieldConverters converts the complete elements collection to roots for the given changed indices.
// If indices is nil, roots for all elements are returned.
func fieldConverters(field types.FieldIndex, elements any, indices []uint64) ([][32]byte, error) {
	if elements == nil {
		return nil, nil
	}

	switch field {
	case types.BlockRoots, types.StateRoots, types.RandaoMixes:
		return convertRoots(indices, elements)
	case types.Eth1DataVotes:
		return convertEth1DataVotes(indices, elements)
	case types.Validators:
		return convertValidators(indices, elements)
	case types.PreviousEpochAttestations, types.CurrentEpochAttestations:
		return convertAttestations(indices, elements)
	case types.Balances:
		return convertBalances(indices, elements)
	default:
		return [][32]byte{}, errors.Errorf("got unsupported type of %v", reflect.TypeOf(elements).Name())
	}
}

func convertRoots(indices []uint64, elements any) ([][32]byte, error) {
	switch castedType := elements.(type) {
	case customtypes.BlockRoots:
		return handle32ByteMVslice(multi_value_slice.BuildEmptyCompositeSlice(castedType), indices)
	case customtypes.StateRoots:
		return handle32ByteMVslice(multi_value_slice.BuildEmptyCompositeSlice(castedType), indices)
	case customtypes.RandaoMixes:
		return handle32ByteMVslice(multi_value_slice.BuildEmptyCompositeSlice(castedType), indices)
	case multi_value_slice.MultiValueSliceComposite[[32]byte]:
		return handle32ByteMVslice(castedType, indices)
	default:
		return nil, errors.Errorf("non-existent type provided %T", castedType)
	}
}

func convertEth1DataVotes(indices []uint64, elements any) ([][32]byte, error) {
	val, ok := elements.([]*ethpb.Eth1Data)
	if !ok {
		return nil, errors.Errorf("Wanted type of %T but got %T", []*ethpb.Eth1Data{}, elements)
	}
	return handleEth1DataSlice(val, indices)
}

func convertValidators(indices []uint64, elements any) ([][32]byte, error) {
	switch casted := elements.(type) {
	case []stateutil.CompactValidator:
		return handleValidatorMVSlice(multi_value_slice.BuildEmptyCompositeSlice(casted), indices)
	case multi_value_slice.MultiValueSliceComposite[stateutil.CompactValidator]:
		return handleValidatorMVSlice(casted, indices)
	default:
		return nil, errors.Errorf("Wanted type of CompactValidator but got %T", elements)
	}
}

func convertAttestations(indices []uint64, elements any) ([][32]byte, error) {
	val, ok := elements.([]*ethpb.PendingAttestation)
	if !ok {
		return nil, errors.Errorf("Wanted type of %T but got %T", []*ethpb.PendingAttestation{}, elements)
	}
	return handlePendingAttestationSlice(val, indices)
}

func convertBalances(indices []uint64, elements any) ([][32]byte, error) {
	switch casted := elements.(type) {
	case []uint64:
		return handleBalanceMVSlice(multi_value_slice.BuildEmptyCompositeSlice(casted), indices)
	case multi_value_slice.MultiValueSliceComposite[uint64]:
		return handleBalanceMVSlice(casted, indices)
	default:
		return nil, errors.Errorf("Wanted type of %T but got %T", []uint64{}, elements)
	}
}

// handle32ByteMVslice computes and returns 32 byte arrays in a slice of root format. This is modified
// to be used with multivalue slices.
func handle32ByteMVslice(
	mv multi_value_slice.MultiValueSliceComposite[[32]byte],
	indices []uint64,
) ([][32]byte, error) {
	count := len(indices)
	state := mv.State()

	// If no indices are provided, we return the roots of the entire slice.
	if count == 0 {
		val := mv.Value(state)
		roots := make([][32]byte, len(val))
		copy(roots, val)

		return roots, nil
	}

	// Otherwise, we return the roots corresponding to the provided indices.
	roots := make([][32]byte, 0, count)
	totalLen := uint64(mv.Len(state))

	for _, index := range indices {
		if index >= totalLen {
			return nil, fmt.Errorf("index %d greater than number of byte arrays %d", index, totalLen)
		}

		val, err := mv.At(state, index)
		if err != nil {
			return nil, fmt.Errorf("at: %w", err)
		}

		roots = append(roots, val)
	}

	return roots, nil
}

// handleValidatorMVSlice returns the validator indices in a slice of root format.
func handleValidatorMVSlice(mv multi_value_slice.MultiValueSliceComposite[stateutil.CompactValidator], indices []uint64) ([][32]byte, error) {
	if len(indices) == 0 {
		return stateutil.OptimizedValidatorRoots(mv.Value(mv.State()))
	}
	roots := make([][32]byte, 0, len(indices))
	totalLen := mv.Len(mv.State())
	for _, idx := range indices {
		if idx >= uint64(totalLen) {
			return nil, fmt.Errorf("index %d greater than number of validators %d", idx, totalLen)
		}
		val, err := mv.At(mv.State(), idx)
		if err != nil {
			return nil, err
		}
		newRoot, err := val.Root()
		if err != nil {
			return nil, err
		}
		roots = append(roots, newRoot)
	}
	return roots, nil
}

// handleEth1DataSlice processes a list of eth1data and indices into the appropriate roots.
func handleEth1DataSlice(val []*ethpb.Eth1Data, indices []uint64) ([][32]byte, error) {
	if len(indices) == 0 {
		roots := make([][32]byte, 0, len(val))
		for _, v := range val {
			root, err := stateutil.Eth1DataRootWithHasher(v)
			if err != nil {
				return nil, err
			}
			roots = append(roots, root)
		}
		return roots, nil
	}
	roots := make([][32]byte, 0, len(indices))
	for _, idx := range indices {
		if idx >= uint64(len(val)) {
			return nil, fmt.Errorf("index %d greater than number of items in eth1 data slice %d", idx, len(val))
		}
		root, err := stateutil.Eth1DataRootWithHasher(val[idx])
		if err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, nil
}

// handlePendingAttestationSlice returns the root of a slice of pending attestations.
func handlePendingAttestationSlice(val []*ethpb.PendingAttestation, indices []uint64) ([][32]byte, error) {
	if len(indices) == 0 {
		roots := make([][32]byte, 0, len(val))
		for _, v := range val {
			root, err := stateutil.PendingAttRootWithHasher(v)
			if err != nil {
				return nil, err
			}
			roots = append(roots, root)
		}
		return roots, nil
	}
	roots := make([][32]byte, 0, len(indices))
	for _, idx := range indices {
		if idx >= uint64(len(val)) {
			return nil, fmt.Errorf("index %d greater than number of pending attestations %d", idx, len(val))
		}
		root, err := stateutil.PendingAttRootWithHasher(val[idx])
		if err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, nil
}

func handleBalanceMVSlice(mv multi_value_slice.MultiValueSliceComposite[uint64], indices []uint64) ([][32]byte, error) {
	if len(indices) == 0 {
		val := mv.Value(mv.State())
		return stateutil.PackUint64IntoChunks(val)
	}
	totalLen := mv.Len(mv.State())
	if totalLen > 0 {
		numOfElems, err := types.Balances.ElemsInChunk()
		if err != nil {
			return nil, err
		}
		iNumOfElems, err := pmath.Int(numOfElems)
		if err != nil {
			return nil, err
		}
		roots := make([][32]byte, 0, len(indices))
		for _, chunkIdx := range indices {
			// indices are chunk-level: chunkIdx maps to the group
			// [chunkIdx*numOfElems .. chunkIdx*numOfElems+numOfElems).
			startGroup := chunkIdx * numOfElems
			var chunk [32]byte
			sizeOfElem := len(chunk) / iNumOfElems
			for i, j := 0, startGroup; j < startGroup+numOfElems; i, j = i+sizeOfElem, j+1 {
				wantedVal := uint64(0)
				if j < uint64(totalLen) {
					val, err := mv.At(mv.State(), j)
					if err != nil {
						return nil, err
					}
					wantedVal = val
				}
				binary.LittleEndian.PutUint64(chunk[i:i+sizeOfElem], wantedVal)
			}
			roots = append(roots, chunk)
		}
		return roots, nil
	}
	return [][32]byte{}, nil
}

func elemCount(elements any) uint64 {
	if elements == nil {
		return 0
	}

	if val, ok := elements.(sliceAccessor); ok {
		return uint64(val.Len(val.State()))
	}

	return uint64(reflect.Indirect(reflect.ValueOf(elements)).Len())
}

func overlayMode(isOverlay bool) trieMode {
	if isOverlay {
		return trieModeOverlay
	}

	return trieModeOwned
}
