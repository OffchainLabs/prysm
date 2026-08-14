package ssz_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	fssz "github.com/prysmaticlabs/fastssz"
)

// variableList builds the SSZ List encoding of variable-size elements: a vector of 4-byte offsets
// followed by the elements.
func variableList(elements ...[]byte) []byte {
	var offsets, data []byte
	offset := 4 * len(elements)
	for _, e := range elements {
		offsets = fssz.WriteOffset(offsets, offset)
		offset += len(e)
		data = append(data, e...)
	}
	return append(offsets, data...)
}

func TestSplitVariableList(t *testing.T) {
	t.Run("splits every element", func(t *testing.T) {
		b := variableList([]byte("a"), []byte("bb"), []byte("ccc"))

		elements, err := ssz.SplitVariableList(b, 10)
		require.NoError(t, err)
		assert.DeepEqual(t, [][]byte{[]byte("a"), []byte("bb"), []byte("ccc")}, elements)
	})
	t.Run("single element", func(t *testing.T) {
		b := variableList([]byte("only"))

		elements, err := ssz.SplitVariableList(b, 10)
		require.NoError(t, err)
		assert.DeepEqual(t, [][]byte{[]byte("only")}, elements)
	})
	t.Run("zero-length elements", func(t *testing.T) {
		b := variableList([]byte{}, []byte{})

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
		b := variableList([]byte("a"), []byte("b"), []byte("c"))

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
