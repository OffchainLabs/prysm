package ssz_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestMarshalVariableList(t *testing.T) {
	t.Run("single element", func(t *testing.T) {
		// One 4-byte offset, so the element starts at 4.
		expected := append([]byte{4, 0, 0, 0}, []byte("only")...)
		assert.DeepEqual(t, expected, ssz.MarshalVariableList([]byte("only")))
	})
	t.Run("several elements", func(t *testing.T) {
		// Three 4-byte offsets, so the elements start at 12, 13 and 15.
		expected := []byte{12, 0, 0, 0, 13, 0, 0, 0, 15, 0, 0, 0}
		expected = append(expected, []byte("abbccc")...)
		assert.DeepEqual(t, expected, ssz.MarshalVariableList([]byte("a"), []byte("bb"), []byte("ccc")))
	})
	t.Run("no elements", func(t *testing.T) {
		assert.Equal(t, 0, len(ssz.MarshalVariableList()))
	})
}

func TestSplitVariableList(t *testing.T) {
	t.Run("splits every element", func(t *testing.T) {
		b := ssz.MarshalVariableList([]byte("a"), []byte("bb"), []byte("ccc"))

		elements, err := ssz.SplitVariableList(b, 10)
		require.NoError(t, err)
		assert.DeepEqual(t, [][]byte{[]byte("a"), []byte("bb"), []byte("ccc")}, elements)
	})
	t.Run("single element", func(t *testing.T) {
		b := ssz.MarshalVariableList([]byte("only"))

		elements, err := ssz.SplitVariableList(b, 10)
		require.NoError(t, err)
		assert.DeepEqual(t, [][]byte{[]byte("only")}, elements)
	})
	t.Run("zero-length elements", func(t *testing.T) {
		b := ssz.MarshalVariableList([]byte{}, []byte{})

		elements, err := ssz.SplitVariableList(b, 10)
		require.NoError(t, err)
		require.Equal(t, 2, len(elements))
		assert.Equal(t, 0, len(elements[0]))
		assert.Equal(t, 0, len(elements[1]))
	})
	t.Run("empty input is an empty list", func(t *testing.T) {
		elements, err := ssz.SplitVariableList(nil, 10)
		require.NoError(t, err)
		assert.Equal(t, 0, len(elements))
	})
	t.Run("element count over maxLength", func(t *testing.T) {
		b := ssz.MarshalVariableList([]byte("a"), []byte("b"), []byte("c"))

		_, err := ssz.SplitVariableList(b, 2)
		assert.NotNil(t, err)
	})
	t.Run("first offset not a multiple of 4", func(t *testing.T) {
		_, err := ssz.SplitVariableList([]byte{5, 0, 0, 0, 1}, 10)
		assert.NotNil(t, err)
	})
	t.Run("truncated offset vector", func(t *testing.T) {
		_, err := ssz.SplitVariableList([]byte{4, 0}, 10)
		assert.NotNil(t, err)
	})
	t.Run("offset past the end of the buffer", func(t *testing.T) {
		// First offset 8 declares two elements; the second offset points well past the 8-byte buffer.
		_, err := ssz.SplitVariableList([]byte{8, 0, 0, 0, 99, 0, 0, 0}, 10)
		assert.NotNil(t, err)
	})
	t.Run("offsets out of order", func(t *testing.T) {
		// Second offset precedes the first, so the first element would have a negative length.
		_, err := ssz.SplitVariableList([]byte{8, 0, 0, 0, 4, 0, 0, 0}, 10)
		assert.NotNil(t, err)
	})
}
