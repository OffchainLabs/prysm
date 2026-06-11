package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestDepositSigCache_GetMiss(t *testing.T) {
	c := NewDepositSigCache()
	got, ok := c.Get([32]byte{0x01})
	require.Equal(t, false, ok)
	require.Equal(t, 0, len(got))
}

func TestDepositSigCache_PutAndGetEmpty(t *testing.T) {
	c := NewDepositSigCache()
	root := [32]byte{0xab}
	c.Put(root, []int{})

	got, ok := c.Get(root)
	require.Equal(t, true, ok)
	require.Equal(t, 0, len(got))
}

func TestDepositSigCache_PutAndGetWithInvalid(t *testing.T) {
	c := NewDepositSigCache()
	root := [32]byte{0xcd}
	c.Put(root, []int{0, 3, 7})

	got, ok := c.Get(root)
	require.Equal(t, true, ok)
	require.DeepEqual(t, []int{0, 3, 7}, got)
}

func TestDepositSigCache_DistinctRoots(t *testing.T) {
	c := NewDepositSigCache()
	r1, r2 := [32]byte{0x01}, [32]byte{0x02}
	c.Put(r1, []int{1})
	c.Put(r2, []int{})

	got1, ok := c.Get(r1)
	require.Equal(t, true, ok)
	require.DeepEqual(t, []int{1}, got1)

	got2, ok := c.Get(r2)
	require.Equal(t, true, ok)
	require.Equal(t, 0, len(got2))
}

func TestDepositSigCache_LRUEviction(t *testing.T) {
	c := NewDepositSigCache()
	for i := range depositSigCacheSize + 1 {
		var root [32]byte
		root[0] = byte(i)
		c.Put(root, []int{i})
	}
	var oldest [32]byte
	oldest[0] = 0
	_, ok := c.Get(oldest)
	require.Equal(t, false, ok)
}
