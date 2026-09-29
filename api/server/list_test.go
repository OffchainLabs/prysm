package server

import (
	"strconv"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// twoBytes is a fixed-size SSZ element: two bytes, the first of which must be zero.
type twoBytes struct {
	b []byte
}

func (*twoBytes) SizeSSZ() int { return 2 }

func (t *twoBytes) UnmarshalSSZ(b []byte) error {
	if b[0] != 0 {
		return errBadElement
	}
	t.b = b
	return nil
}

var errBadElement = strconv.ErrSyntax

func TestConvertList(t *testing.T) {
	src := []string{"1", "bad", "3"}
	dst, failures := ConvertList(src, strconv.Atoi)

	require.Equal(t, 3, len(dst))
	assert.DeepEqual(t, []int{1, 0, 3}, dst)
	require.Equal(t, 1, len(failures))
	assert.Equal(t, 1, failures[0].Index)
}

func TestDecodeJSONList(t *testing.T) {
	type item struct {
		N string `json:"n"`
	}
	convert := func(i *item) (int, error) { return strconv.Atoi(i.N) }

	t.Run("index-aligned failures", func(t *testing.T) {
		items, failures, err := DecodeJSONList(strings.NewReader(`[{"n":"1"},{"n":"x"},{"n":"3"}]`), convert)
		require.NoError(t, err)
		assert.DeepEqual(t, []int{1, 0, 3}, items)
		require.Equal(t, 1, len(failures))
		assert.Equal(t, 1, failures[0].Index)
	})
	t.Run("null element does not reach the converter", func(t *testing.T) {
		panicking := func(i *item) (int, error) { return strconv.Atoi(i.N) }
		items, failures, err := DecodeJSONList(strings.NewReader(`[null,{"n":"2"}]`), panicking)
		require.NoError(t, err)
		assert.DeepEqual(t, []int{0, 2}, items)
		require.Equal(t, 1, len(failures))
		assert.Equal(t, 0, failures[0].Index)
		assert.Equal(t, "null element", failures[0].Message)
	})
	t.Run("empty array", func(t *testing.T) {
		items, failures, err := DecodeJSONList(strings.NewReader(`[]`), convert)
		require.NoError(t, err)
		assert.Equal(t, 0, len(items))
		assert.Equal(t, 0, len(failures))
	})
	t.Run("malformed body", func(t *testing.T) {
		_, _, err := DecodeJSONList(strings.NewReader(`{`), convert)
		require.NotNil(t, err)
	})
}

func TestDecodeSSZList(t *testing.T) {
	t.Run("index-aligned failures", func(t *testing.T) {
		items, failures, err := DecodeSSZList[twoBytes](strings.NewReader("\x00\x01\xff\x02\x00\x03"))
		require.NoError(t, err)
		require.Equal(t, 3, len(items))
		assert.Equal(t, uint8(1), items[0].b[1])
		assert.Equal(t, true, items[1] == nil)
		assert.Equal(t, uint8(3), items[2].b[1])
		require.Equal(t, 1, len(failures))
		assert.Equal(t, 1, failures[0].Index)
		assert.StringContains(t, "could not decode SSZ message", failures[0].Message)
	})
	t.Run("empty body yields zero items", func(t *testing.T) {
		items, failures, err := DecodeSSZList[twoBytes](strings.NewReader(""))
		require.NoError(t, err)
		assert.Equal(t, 0, len(items))
		assert.Equal(t, 0, len(failures))
	})
	t.Run("misaligned body", func(t *testing.T) {
		_, _, err := DecodeSSZList[twoBytes](strings.NewReader("\x00"))
		assert.ErrorContains(t, "invalid SSZ list size", err)
	})
}
