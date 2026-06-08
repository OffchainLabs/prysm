package ssz_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

type staticHashable struct {
	root [32]byte
	err  error
}

func (s staticHashable) HashTreeRoot() ([32]byte, error) {
	if s.err != nil {
		return [32]byte{}, s.err
	}
	return s.root, nil
}

func chunkFromIndex(i int) [32]byte {
	var out [32]byte
	binary.LittleEndian.PutUint64(out[:8], uint64(i+1))
	return out
}

func hashPair(left, right [32]byte) [32]byte {
	var input [64]byte
	copy(input[:32], left[:])
	copy(input[32:], right[:])
	return hash.Hash(input[:])
}

func referenceMerkleizeProgressive(chunks [][32]byte, numLeaves uint64) [32]byte {
	if len(chunks) == 0 {
		return [32]byte{}
	}
	if numLeaves == 0 {
		panic("numLeaves must be positive")
	}

	take := len(chunks)
	if uint64(take) > numLeaves {
		take = int(numLeaves)
	}
	left := slices.Clone(chunks[:take])
	a := ssz.MerkleizeVector(left, numLeaves)
	b := referenceMerkleizeProgressive(chunks[take:], numLeaves*4)
	return hashPair(a, b)
}

func TestMerkleizeProgressiveChunks_MatchesReference(t *testing.T) {
	testCases := []int{0, 1, 2, 3, 5, 6, 21, 22, 85, 86}

	for _, n := range testCases {
		t.Run(fmt.Sprintf("len_%d", n), func(t *testing.T) {
			chunks := make([][32]byte, n)
			for i := range chunks {
				chunks[i] = chunkFromIndex(i)
			}

			expected := referenceMerkleizeProgressive(chunks, 1)
			actual := ssz.MerkleizeProgressiveChunks(chunks)
			require.Equal(t, expected, actual)
		})
	}
}

func TestMerkleizeVectorSSZProgressive(t *testing.T) {
	elements := []staticHashable{
		{root: chunkFromIndex(0)},
		{root: chunkFromIndex(1)},
		{root: chunkFromIndex(2)},
	}

	root, err := ssz.MerkleizeVectorSSZProgressive(elements)
	require.NoError(t, err)

	expected := ssz.MerkleizeProgressiveChunks([][32]byte{
		elements[0].root,
		elements[1].root,
		elements[2].root,
	})
	require.Equal(t, expected, root)
}

func TestMerkleizeVectorSSZProgressive_Error(t *testing.T) {
	e := errors.New("merkleError")
	elements := []staticHashable{{root: chunkFromIndex(0)}, {err: e}}

	_, err := ssz.MerkleizeVectorSSZProgressive(elements)
	require.ErrorContains(t, "merkleError", err)
}

func TestMerkleizeListSSZProgressive(t *testing.T) {
	elements := []staticHashable{
		{root: chunkFromIndex(10)},
		{root: chunkFromIndex(11)},
		{root: chunkFromIndex(12)},
	}

	got, err := ssz.MerkleizeListSSZProgressive(elements)
	require.NoError(t, err)

	body := ssz.MerkleizeProgressiveChunks([][32]byte{
		elements[0].root,
		elements[1].root,
		elements[2].root,
	})
	var length [32]byte
	binary.LittleEndian.PutUint64(length[:8], uint64(len(elements)))
	expected := ssz.MixInLength(body, length[:])
	require.Equal(t, expected, got)
}

func TestSliceRootProgressive(t *testing.T) {
	elements := []staticHashable{{root: chunkFromIndex(0)}, {root: chunkFromIndex(1)}}

	sliceRoot, err := ssz.SliceRootProgressive(elements)
	require.NoError(t, err)
	listRoot, err := ssz.MerkleizeListSSZProgressive(elements)
	require.NoError(t, err)
	require.Equal(t, listRoot, sliceRoot)
}

func TestByteSliceRootProgressive(t *testing.T) {
	testCases := [][]byte{
		nil,
		{},
		{0x01},
		{0x01, 0x02, 0x03},
		make([]byte, 32),
		make([]byte, 33),
	}
	for _, input := range testCases {
		t.Run(fmt.Sprintf("len_%d", len(input)), func(t *testing.T) {
			got, err := ssz.ByteSliceRootProgressive(input)
			require.NoError(t, err)

			var chunks [][32]byte
			if len(input) > 0 {
				chunks, err = ssz.PackByChunk([][]byte{input})
				require.NoError(t, err)
			}
			body := ssz.MerkleizeProgressiveChunks(chunks)
			var length [32]byte
			binary.LittleEndian.PutUint64(length[:8], uint64(len(input)))
			expected := ssz.MixInLength(body, length[:])
			require.Equal(t, expected, got)
		})
	}
}

func TestByteSliceRootProgressive_EmptyReferenceRoot(t *testing.T) {
	for _, input := range [][]byte{nil, {}} {
		got, err := ssz.ByteSliceRootProgressive(input)
		require.NoError(t, err)
		require.Equal(t, "f5a5fd42d16a20302798ef6ed309979b43003d2320d9f0e8ea9831a92759fb4b", fmt.Sprintf("%x", got))
	}
}

func TestContainerRootProgressive(t *testing.T) {
	fieldRoots := [][32]byte{
		chunkFromIndex(0),
		chunkFromIndex(1),
	}
	activeFields := []bool{true, false, true}

	got, err := ssz.ContainerRootProgressive(fieldRoots, activeFields)
	require.NoError(t, err)

	body := ssz.MerkleizeProgressiveChunks(fieldRoots)
	expected, err := ssz.MixInActiveFields(body, activeFields)
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func TestContainerRootProgressive_EmptyActiveFields(t *testing.T) {
	_, err := ssz.ContainerRootProgressive([][32]byte{}, []bool{})
	require.ErrorContains(t, "active fields cannot be empty", err)
}

func TestContainerRootProgressive_ActiveFieldsExceedsLimit(t *testing.T) {
	af := make([]bool, 257)
	_, err := ssz.ContainerRootProgressive([][32]byte{}, af)
	require.ErrorContains(t, "exceeds maximum", err)
}

func TestContainerRootProgressive_ActiveCountMismatch(t *testing.T) {
	fieldRoots := [][32]byte{
		chunkFromIndex(0),
		chunkFromIndex(1),
	}
	_, err := ssz.ContainerRootProgressive(fieldRoots, []bool{true, false})
	require.ErrorContains(t, "active fields count", err)
}

func TestMixInActiveFields(t *testing.T) {
	root := chunkFromIndex(42)
	activeFields := []bool{true, false, true, true, false, false, false, true, true}

	got, err := ssz.MixInActiveFields(root, activeFields)
	require.NoError(t, err)

	var packed [32]byte
	packed[0] = 0b10001101
	packed[1] = 0b00000001
	expected := hashPair(root, packed)
	require.Equal(t, expected, got)
}

func TestMixInActiveFields_TooMany(t *testing.T) {
	activeFields := make([]bool, 257)
	_, err := ssz.MixInActiveFields(chunkFromIndex(0), activeFields)
	require.ErrorContains(t, "exceeds maximum 256", err)
}
