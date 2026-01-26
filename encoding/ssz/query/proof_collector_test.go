package query

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ssz "github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	sszquerypb "github.com/OffchainLabs/prysm/v7/proto/ssz_query/testing"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProofCollector_New(t *testing.T) {
	pc := newProofCollector()

	require.NotNil(t, pc)
	require.Equal(t, 0, len(pc.requiredSiblings))
	require.Equal(t, 0, len(pc.requiredLeaves))
	require.Equal(t, 0, len(pc.siblings))
	require.Equal(t, 0, len(pc.leaves))
}

func TestProofCollector_Reset(t *testing.T) {
	pc := newProofCollector()
	pc.requiredSiblings[3] = struct{}{}
	pc.requiredLeaves[5] = struct{}{}
	pc.siblings[3] = [32]byte{1}
	pc.leaves[5] = [32]byte{2}

	pc.reset()

	require.Equal(t, 0, len(pc.requiredSiblings))
	require.Equal(t, 0, len(pc.requiredLeaves))
	require.Equal(t, 0, len(pc.siblings))
	require.Equal(t, 0, len(pc.leaves))
}

func TestProofCollector_AddTarget(t *testing.T) {
	pc := newProofCollector()
	pc.addTarget(5)

	_, hasLeaf := pc.requiredLeaves[5]
	_, hasSibling4 := pc.requiredSiblings[4]
	_, hasSibling3 := pc.requiredSiblings[3]
	_, hasSibling1 := pc.requiredSiblings[1] // GI 1 is the root

	require.Equal(t, true, hasLeaf)
	require.Equal(t, true, hasSibling4)
	require.Equal(t, true, hasSibling3)
	require.Equal(t, false, hasSibling1)
}

func TestProofCollector_ToProof(t *testing.T) {
	pc := newProofCollector()
	pc.addTarget(5)

	leaf := [32]byte{9}
	sibling4 := [32]byte{4}
	sibling3 := [32]byte{3}

	pc.collectLeaf(5, leaf)
	pc.collectSibling(4, sibling4)
	pc.collectSibling(3, sibling3)

	proof, err := pc.toProof()
	require.NoError(t, err)

	require.Equal(t, 5, proof.Index)
	require.DeepEqual(t, leaf[:], proof.Leaf)
	require.Equal(t, 2, len(proof.Hashes))
	require.DeepEqual(t, sibling4[:], proof.Hashes[0])
	require.DeepEqual(t, sibling3[:], proof.Hashes[1])
}

func TestProofCollector_ToProof_NoLeaves(t *testing.T) {
	pc := newProofCollector()
	_, err := pc.toProof()
	require.NotNil(t, err)
}

func TestProofCollector_AddTarget_RegisterRequiredSiblings(t *testing.T) {
	pc := newProofCollector()
	pc.addTarget(6)

	pc.registerRequiredSiblings(5)

	_, hasLeaf5 := pc.requiredLeaves[5]
	_, hasLeaf6 := pc.requiredLeaves[6] // this target was reseted by calling pc.registerRequiredSiblings(5)
	_, hasSibling4 := pc.requiredSiblings[4]
	_, hasSibling3 := pc.requiredSiblings[3]
	_, hasSibling7 := pc.requiredSiblings[7]

	require.Equal(t, true, hasLeaf5)
	require.Equal(t, false, hasLeaf6)
	require.Equal(t, true, hasSibling4)
	require.Equal(t, true, hasSibling3)
	require.Equal(t, false, hasSibling7)
}

func TestProofCollector_CollectLeaf(t *testing.T) {
	pc := newProofCollector()
	leaf := [32]byte{7}

	pc.collectLeaf(10, leaf)
	require.Equal(t, 0, len(pc.leaves))

	pc.addTarget(10)
	pc.collectLeaf(10, leaf)
	stored, ok := pc.leaves[10]
	require.Equal(t, true, ok)
	require.Equal(t, leaf, stored)
}

func TestProofCollector_CollectSibling(t *testing.T) {
	pc := newProofCollector()
	hash := [32]byte{5}

	pc.collectSibling(4, hash)
	require.Equal(t, 0, len(pc.siblings))

	pc.addTarget(5)
	pc.collectSibling(4, hash)
	stored, ok := pc.siblings[4]
	require.Equal(t, true, ok)
	require.Equal(t, hash, stored)
}

func TestIsStrictAncestor(t *testing.T) {
	testCases := []struct {
		ancestor   uint64
		descendant uint64
		expected   bool
	}{
		{1, 1, false}, // same node - NOT a strict ancestor
		{2, 2, false}, // same node - NOT a strict ancestor
		{1, 2, true},  // root is strict ancestor of gindex 2
		{1, 3, true},  // root is strict ancestor of gindex 3
		{1, 15, true}, // root is strict ancestor of deep node
		{2, 4, true},  // 2 is parent of 4
		{2, 5, true},  // 2 is parent of 5
		{2, 8, true},  // 2 is strict ancestor of 8 (grandchild)
		{2, 3, false}, // 2 is not ancestor of 3 (sibling)
		{2, 6, false}, // 2 is not ancestor of 6 (nephew)
		{3, 2, false}, // 3 is not ancestor of 2
		{4, 2, false}, // 4 is not ancestor of 2 (child checking parent)
		{0, 1, false}, // invalid ancestor
		{1, 0, false}, // invalid descendant
	}

	for _, tc := range testCases {
		result := isStrictAncestor(tc.ancestor, tc.descendant)
		require.Equal(t, tc.expected, result, "isStrictAncestor(%d, %d)", tc.ancestor, tc.descendant)
	}
}

func TestProofCollector_HasTargetsInSubtree(t *testing.T) {
	t.Run("no targets", func(t *testing.T) {
		pc := newProofCollector()
		require.Equal(t, false, pc.hasTargetsInSubtree(2))
	})

	t.Run("target outside subtree", func(t *testing.T) {
		pc := newProofCollector()
		pc.addTarget(3) // sibling of subtree root 2; sibling 2 is needed but it's the root, not inside
		require.Equal(t, false, pc.hasTargetsInSubtree(2))
	})

	t.Run("target at subtree root", func(t *testing.T) {
		pc := newProofCollector()
		pc.addTarget(2) // target is the subtree root itself, not strictly inside
		require.Equal(t, false, pc.hasTargetsInSubtree(2))
	})

	t.Run("target inside subtree (leaf)", func(t *testing.T) {
		pc := newProofCollector()
		pc.addTarget(9) // gindex 9 = 4*2+1 is child of 4, which is child of 2
		require.Equal(t, true, pc.hasTargetsInSubtree(2))
	})

	t.Run("required sibling inside subtree", func(t *testing.T) {
		pc := newProofCollector()
		pc.addTarget(8) // gindex 8 needs sibling 9 (both strictly under subtree 2)
		// addTarget(8) registers leaf 8 and siblings 9, 5, 3
		require.Equal(t, true, pc.hasTargetsInSubtree(2))  // 8 and 9 are strictly inside subtree 2
		require.Equal(t, false, pc.hasTargetsInSubtree(3)) // sibling 3 is the root of subtree 3, not inside
		require.Equal(t, false, pc.hasTargetsInSubtree(6)) // nothing in subtree 6
	})
}

func TestProofCollector_Merkleize_BasicTypes(t *testing.T) {
	testCases := []struct {
		name     string
		sszType  SSZType
		value    any
		expected [32]byte
	}{
		{
			name:    "uint8",
			sszType: Uint8,
			value:   uint8(0x11),
			expected: func() [32]byte {
				var leaf [32]byte
				leaf[0] = 0x11
				return leaf
			}(),
		},
		{
			name:    "uint16",
			sszType: Uint16,
			value:   uint16(0x2211),
			expected: func() [32]byte {
				var leaf [32]byte
				binary.LittleEndian.PutUint16(leaf[:2], 0x2211)
				return leaf
			}(),
		},
		{
			name:    "uint32",
			sszType: Uint32,
			value:   uint32(0x44332211),
			expected: func() [32]byte {
				var leaf [32]byte
				binary.LittleEndian.PutUint32(leaf[:4], 0x44332211)
				return leaf
			}(),
		},
		{
			name:    "uint64",
			sszType: Uint64,
			value:   uint64(0x8877665544332211),
			expected: func() [32]byte {
				var leaf [32]byte
				binary.LittleEndian.PutUint64(leaf[:8], 0x8877665544332211)
				return leaf
			}(),
		},
		{
			name:    "bool",
			sszType: Boolean,
			value:   true,
			expected: func() [32]byte {
				var leaf [32]byte
				leaf[0] = 1
				return leaf
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pc := newProofCollector()
			gindex := uint64(3)
			pc.addTarget(gindex)

			leaf, err := pc.merkleizeBasicType(tc.sszType, reflect.ValueOf(tc.value), gindex)
			require.NoError(t, err)
			require.Equal(t, tc.expected, leaf)

			stored, ok := pc.leaves[gindex]
			require.Equal(t, true, ok)
			require.Equal(t, tc.expected, stored)
		})
	}
}

func TestProofCollector_Merkleize_Container(t *testing.T) {
	container := makeFixedTestContainer(0x01)

	info, err := AnalyzeObject(container)
	require.NoError(t, err)

	pc := newProofCollector()
	pc.addTarget(1)

	root, err := pc.merkleize(info, reflect.ValueOf(container), 1)
	require.NoError(t, err)

	expected, err := container.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, expected, root)

	stored, ok := pc.leaves[1]
	require.Equal(t, true, ok)
	require.Equal(t, expected, stored)
}

func TestProofCollector_Merkleize_Vector(t *testing.T) {
	container := makeFixedTestContainer(0x02)
	info, err := AnalyzeObject(container)
	require.NoError(t, err)

	ci, err := info.ContainerInfo()
	require.NoError(t, err)
	field := ci.fields["vector_field"]

	pc := newProofCollector()
	root, err := pc.merkleizeVector(field.sszInfo, reflect.ValueOf(container.VectorField), 1)
	require.NoError(t, err)

	serialized := make([][]byte, len(container.VectorField))
	for i, v := range container.VectorField {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, v)
		serialized[i] = buf
	}
	chunks, err := ssz.PackByChunk(serialized)
	require.NoError(t, err)
	limit, err := getChunkCount(field.sszInfo)
	require.NoError(t, err)
	expected := ssz.MerkleizeVector(chunks, limit)

	require.Equal(t, expected, root)
}

func TestProofCollector_Merkleize_List(t *testing.T) {
	list := []*sszquerypb.FixedNestedContainer{
		makeFixedNestedContainer(1, 0x10),
		makeFixedNestedContainer(2, 0x20),
	}
	container := makeVariableTestContainer(list, bitfield.NewBitlist(1))
	info, err := AnalyzeObject(container)
	require.NoError(t, err)

	ci, err := info.ContainerInfo()
	require.NoError(t, err)
	field := ci.fields["field_list_container"]

	pc := newProofCollector()
	root, err := pc.merkleizeList(field.sszInfo, reflect.ValueOf(list), 1)
	require.NoError(t, err)

	listInfo, err := field.sszInfo.ListInfo()
	require.NoError(t, err)
	expected, err := ssz.MerkleizeListSSZ(list, listInfo.Limit())
	require.NoError(t, err)

	require.Equal(t, expected, root)
}

func TestProofCollector_Merkleize_Bitvector(t *testing.T) {
	container := makeFixedTestContainer(0x03)
	info, err := AnalyzeObject(container)
	require.NoError(t, err)

	ci, err := info.ContainerInfo()
	require.NoError(t, err)
	field := ci.fields["bitvector64_field"]

	pc := newProofCollector()
	root, err := pc.merkleizeBitvector(field.sszInfo, reflect.ValueOf(container.Bitvector64Field), 1)
	require.NoError(t, err)

	expected, err := ssz.MerkleizeByteSliceSSZ([]byte(container.Bitvector64Field))
	require.NoError(t, err)
	require.Equal(t, expected, root)
}

func TestProofCollector_Merkleize_Bitlist(t *testing.T) {
	bitlist := bitfield.NewBitlist(16)
	bitlist.SetBitAt(3, true)
	bitlist.SetBitAt(8, true)

	container := makeVariableTestContainer(nil, bitlist)
	info, err := AnalyzeObject(container)
	require.NoError(t, err)

	ci, err := info.ContainerInfo()
	require.NoError(t, err)
	field := ci.fields["bitlist_field"]

	pc := newProofCollector()
	root, err := pc.merkleizeBitlist(field.sszInfo, reflect.ValueOf(container.BitlistField), 1)
	require.NoError(t, err)

	bitlistInfo, err := field.sszInfo.BitlistInfo()
	require.NoError(t, err)
	expected, err := ssz.BitlistRoot(bitfield.Bitlist(bitlist), bitlistInfo.Limit())
	require.NoError(t, err)
	require.Equal(t, expected, root)
}

func TestProofCollector_MerkleizeVectorBody_Basic(t *testing.T) {
	container := makeFixedTestContainer(0x04)
	info, err := AnalyzeObject(container)
	require.NoError(t, err)

	ci, err := info.ContainerInfo()
	require.NoError(t, err)
	field := ci.fields["vector_field"]
	vectorInfo, err := field.sszInfo.VectorInfo()
	require.NoError(t, err)
	length := len(container.VectorField)
	limit, err := getChunkCount(field.sszInfo)
	require.NoError(t, err)

	pc := newProofCollector()
	root, err := pc.merkleizeVectorBody(vectorInfo.element, reflect.ValueOf(container.VectorField), length, limit, 2)
	require.NoError(t, err)

	serialized := make([][]byte, len(container.VectorField))
	for i, v := range container.VectorField {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, v)
		serialized[i] = buf
	}
	chunks, err := ssz.PackByChunk(serialized)
	require.NoError(t, err)
	expected := ssz.MerkleizeVector(chunks, limit)

	require.Equal(t, expected, root)
}

func TestProofCollector_MerkleizeVectorAndCollect(t *testing.T) {
	pc := newProofCollector()
	pc.addTarget(6)

	elements := [][32]byte{{1}, {2}}
	expected := ssz.MerkleizeVector(append([][32]byte{}, elements...), 2)
	root := pc.merkleizeVectorAndCollect(elements, 3, 1)

	storedLeaf, hasLeaf := pc.leaves[6]
	storedSibling, hasSibling := pc.siblings[7]

	require.Equal(t, true, hasLeaf)
	require.Equal(t, true, hasSibling)
	require.Equal(t, elements[0], storedLeaf)
	require.Equal(t, elements[1], storedSibling)

	require.Equal(t, expected, root)
}

func TestProofCollector_MixinLengthAndCollect(t *testing.T) {
	list := []*sszquerypb.FixedNestedContainer{
		makeFixedNestedContainer(1, 0x10),
		makeFixedNestedContainer(2, 0x20),
	}
	container := makeVariableTestContainer(list, bitfield.NewBitlist(1))
	info, err := AnalyzeObject(container)
	require.NoError(t, err)

	ci, err := info.ContainerInfo()
	require.NoError(t, err)
	field := ci.fields["field_list_container"]

	// Target gindex 2 (data root) - sibling at gindex 3 (length hash) should be collected
	pc := newProofCollector()
	pc.addTarget(2)
	root, err := pc.merkleizeList(field.sszInfo, reflect.ValueOf(list), 1)
	require.NoError(t, err)

	listInfo, err := field.sszInfo.ListInfo()
	require.NoError(t, err)
	expected, err := ssz.MerkleizeListSSZ(list, listInfo.Limit())
	require.NoError(t, err)
	require.Equal(t, expected, root)

	// Verify data root is collected as leaf at gindex 2
	storedLeaf, hasLeaf := pc.leaves[2]
	require.Equal(t, true, hasLeaf)

	// Verify length hash is collected as sibling at gindex 3
	storedSibling, hasSibling := pc.siblings[3]
	require.Equal(t, true, hasSibling)

	// Verify the root is hash(dataRoot || lengthHash)
	expectedBuf := append(storedLeaf[:], storedSibling[:]...)
	expectedRoot := sha256.Sum256(expectedBuf)
	require.Equal(t, expectedRoot, root)
}

func TestProofCollector_OptimizedContainerRoots(t *testing.T) {
	containers := []*sszquerypb.FixedNestedContainer{
		makeFixedNestedContainer(1, 0x05),
		makeFixedNestedContainer(2, 0x06),
	}
	info, err := AnalyzeObject(containers[0])
	require.NoError(t, err)

	pc := newProofCollector()
	roots, err := pc.optimizedContainerRoots(info, reflect.ValueOf(containers))
	require.NoError(t, err)

	require.Equal(t, len(containers), len(roots))
	for i, c := range containers {
		expected, err := c.HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, expected, roots[i])
	}
}

func TestProofCollector_HashContainerHelper(t *testing.T) {
	containers := []*sszquerypb.FixedTestContainer{
		makeFixedTestContainer(0x07),
		makeFixedTestContainer(0x08),
	}
	info, err := AnalyzeObject(containers[0])
	require.NoError(t, err)

	ci, err := info.ContainerInfo()
	require.NoError(t, err)

	pc := newProofCollector()
	containerFieldRoots := len(ci.order)
	roots := make([][32]byte, len(containers)*containerFieldRoots)

	ctx := context.Background()
	fn := pc.hashContainerHelper(ctx, ci, reflect.ValueOf(containers), roots, 0, 1, containerFieldRoots)
	err = fn()
	require.NoError(t, err)

	expected, err := pc.containerFieldRoots(ci, reflect.ValueOf(containers[0]))
	require.NoError(t, err)

	for i := range expected {
		require.Equal(t, expected[i], roots[i])
	}
}

func TestProofCollector_ContainerFieldRoots(t *testing.T) {
	container := makeFixedTestContainer(0x09)
	info, err := AnalyzeObject(container)
	require.NoError(t, err)

	ci, err := info.ContainerInfo()
	require.NoError(t, err)

	pc := newProofCollector()
	fieldRoots, err := pc.containerFieldRoots(ci, reflect.ValueOf(container))
	require.NoError(t, err)
	require.Equal(t, len(ci.order), len(fieldRoots))

	computed := ssz.MerkleizeVector(fieldRoots, uint64(len(fieldRoots)))
	expected, err := container.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, expected, computed)
}

// Test to ensure that optimized container roots from ProofCollector
// match those from the stateutil.OptimizedValidatorRoots function.
func TestOptimizedContainerRootsMatchesValidatorRoots(t *testing.T) {
	validators := make([]*ethpb.Validator, 16)
	for i := range validators {
		validators[i] = makeTestValidator(i)
	}

	info, err := AnalyzeObject(validators[0])
	require.NoError(t, err)

	pc := newProofCollector()
	roots, err := pc.optimizedContainerRoots(info, reflect.ValueOf(validators))
	require.NoError(t, err)

	expected, err := stateutil.OptimizedValidatorRoots(validators)
	require.NoError(t, err)

	require.Equal(t, len(expected), len(roots))
	for i := range expected {
		require.Equal(t, expected[i], roots[i])
	}
}

// Benchmark tests for ProofCollector
func BenchmarkOptimizedContainerRoots(b *testing.B) {
	validators := make([]*ethpb.Validator, 1000)
	for i := range validators {
		validators[i] = makeTestValidator(i)
	}

	info, err := AnalyzeObject(validators[0])
	require.NoError(b, err)

	pc := newProofCollector()
	v := reflect.ValueOf(validators)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pc.optimizedContainerRoots(info, v)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOptimizedValidatorRoots(b *testing.B) {
	validators := make([]*ethpb.Validator, 1000)
	for i := range validators {
		validators[i] = makeTestValidator(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := stateutil.OptimizedValidatorRoots(validators)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProofCollectorMerkleize(b *testing.B) {
	validators := make([]*ethpb.Validator, 1000)
	for i := range validators {
		validators[i] = makeTestValidator(i)
	}

	info, err := AnalyzeObject(validators[0])
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, val := range validators {
			pc := newProofCollector()
			v := reflect.ValueOf(val)
			_, err := pc.merkleize(info, v, 1)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func makeTestValidator(i int) *ethpb.Validator {
	pubkey := make([]byte, 48)
	for j := range pubkey {
		pubkey[j] = byte(i + j)
	}

	withdrawalCredentials := make([]byte, 32)
	for j := range withdrawalCredentials {
		withdrawalCredentials[j] = byte(255 - ((i + j) % 256))
	}

	return &ethpb.Validator{
		PublicKey:                  pubkey,
		WithdrawalCredentials:      withdrawalCredentials,
		EffectiveBalance:           uint64(32000000000 + i),
		Slashed:                    i%2 == 0,
		ActivationEligibilityEpoch: primitives.Epoch(i),
		ActivationEpoch:            primitives.Epoch(i + 1),
		ExitEpoch:                  primitives.Epoch(i + 2),
		WithdrawableEpoch:          primitives.Epoch(i + 3),
	}
}

func makeFixedNestedContainer(value uint64, seed byte) *sszquerypb.FixedNestedContainer {
	value2 := make([]byte, 32)
	for i := range value2 {
		value2[i] = seed + byte(i)
	}
	return &sszquerypb.FixedNestedContainer{
		Value1: value,
		Value2: value2,
	}
}

func makeFixedTestContainer(seed byte) *sszquerypb.FixedTestContainer {
	fieldBytes32 := make([]byte, 32)
	for i := range fieldBytes32 {
		fieldBytes32[i] = seed + byte(i)
	}

	vectorField := make([]uint64, 24)
	for i := range vectorField {
		vectorField[i] = uint64(seed) + uint64(i)
	}

	rows := make([][]byte, 5)
	for i := range rows {
		row := make([]byte, 32)
		for j := range row {
			row[j] = seed + byte(i) + byte(j)
		}
		rows[i] = row
	}

	bitvector64 := bitfield.NewBitvector64()
	bitvector64.SetBitAt(1, true)
	bitvector512 := bitfield.NewBitvector512()
	bitvector512.SetBitAt(10, true)

	trailing := make([]byte, 56)
	for i := range trailing {
		trailing[i] = seed + byte(i)
	}

	return &sszquerypb.FixedTestContainer{
		FieldUint32:            uint32(seed) + 1,
		FieldUint64:            uint64(seed) + 2,
		FieldBool:              true,
		FieldBytes32:           fieldBytes32,
		Nested:                 makeFixedNestedContainer(uint64(seed)+3, seed),
		VectorField:            vectorField,
		TwoDimensionBytesField: rows,
		Bitvector64Field:       bitvector64,
		Bitvector512Field:      bitvector512,
		TrailingField:          trailing,
	}
}

func makeVariableTestContainer(list []*sszquerypb.FixedNestedContainer, bitlist bitfield.Bitlist) *sszquerypb.VariableTestContainer {
	leading := make([]byte, 32)
	for i := range leading {
		leading[i] = byte(i)
	}
	trailing := make([]byte, 56)
	for i := range trailing {
		trailing[i] = byte(255 - i)
	}

	if bitlist == nil {
		bitlist = bitfield.NewBitlist(0)
	}

	return &sszquerypb.VariableTestContainer{
		LeadingField:       leading,
		FieldListContainer: list,
		BitlistField:       bitlist,
		TrailingField:      trailing,
	}
}
