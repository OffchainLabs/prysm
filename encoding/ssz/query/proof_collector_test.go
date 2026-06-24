package query

import (
	"crypto/sha256"
	"encoding/binary"
	"reflect"
	"slices"
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
	container := makeFixedTestContainer()

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
	container := makeFixedTestContainer()
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
		makeFixedNestedContainer(1),
		makeFixedNestedContainer(2),
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
	container := makeFixedTestContainer()
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
	container := makeFixedTestContainer()
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
	expected := ssz.MerkleizeVector(slices.Clone(elements), 2)
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
		makeFixedNestedContainer(1),
		makeFixedNestedContainer(2),
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

func BenchmarkOptimizedValidatorRoots(b *testing.B) {
	validators := make([]*ethpb.Validator, 1000)
	for i := range validators {
		validators[i] = makeTestValidator(i)
	}

	compactValidators := stateutil.CompactValidatorsFromProto(validators)

	b.ResetTimer()
	for b.Loop() {
		_, err := stateutil.OptimizedValidatorRoots(compactValidators)
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
	for b.Loop() {
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

func makeFixedNestedContainer(value uint64) *sszquerypb.FixedNestedContainer {
	value2 := make([]byte, 32)
	for i := range value2 {
		value2[i] = byte(i)
	}
	return &sszquerypb.FixedNestedContainer{
		Value1: value,
		Value2: value2,
	}
}

func makeFixedTestContainer() *sszquerypb.FixedTestContainer {
	fieldBytes32 := make([]byte, 32)
	for i := range fieldBytes32 {
		fieldBytes32[i] = byte(i)
	}

	vectorField := make([]uint64, 24)
	for i := range vectorField {
		vectorField[i] = uint64(i)
	}

	rows := make([][]byte, 5)
	for i := range rows {
		row := make([]byte, 32)
		for j := range row {
			row[j] = byte(i) + byte(j)
		}
		rows[i] = row
	}

	bitvector64 := bitfield.NewBitvector64()
	bitvector64.SetBitAt(1, true)
	bitvector512 := bitfield.NewBitvector512()
	bitvector512.SetBitAt(10, true)

	trailing := make([]byte, 56)
	for i := range trailing {
		trailing[i] = byte(i)
	}

	return &sszquerypb.FixedTestContainer{
		FieldUint32:            1,
		FieldUint64:            2,
		FieldBool:              true,
		FieldBytes32:           fieldBytes32,
		Nested:                 makeFixedNestedContainer(3),
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
