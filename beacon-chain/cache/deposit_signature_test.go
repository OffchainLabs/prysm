package cache

import (
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestDepositSignatureCache_GetMiss(t *testing.T) {
	c := newDepositSignatureCache()
	valid, ok := c.Get([32]byte{0x01})
	require.Equal(t, false, ok)
	require.Equal(t, false, valid)
	require.Equal(t, false, c.Has([32]byte{0x01}))
}

func TestDepositSignatureCache_PutAndGet(t *testing.T) {
	c := newDepositSignatureCache()
	c.Put([32]byte{0xaa}, true)
	c.Put([32]byte{0xbb}, false)

	valid, ok := c.Get([32]byte{0xaa})
	require.Equal(t, true, ok)
	require.Equal(t, true, valid)

	valid, ok = c.Get([32]byte{0xbb})
	require.Equal(t, true, ok)
	require.Equal(t, false, valid)

	require.Equal(t, true, c.Has([32]byte{0xbb}))
	require.Equal(t, 2, c.Len())
}

func TestDepositSignatureCache_Clear(t *testing.T) {
	c := newDepositSignatureCache()
	c.Put([32]byte{0x01}, true)
	require.Equal(t, 1, c.Len())
	c.Clear()
	require.Equal(t, 0, c.Len())
	_, ok := c.Get([32]byte{0x01})
	require.Equal(t, false, ok)
}

func TestDepositSignatureCache_RefusesAboveCapWithoutEviction(t *testing.T) {
	c := newDepositSignatureCache()
	for i := range depositSignatureCacheCap {
		var key [32]byte
		key[0] = byte(i)
		key[1] = byte(i >> 8)
		key[2] = byte(i >> 16)
		c.Put(key, true)
	}
	require.Equal(t, depositSignatureCacheCap, c.Len())

	var overflow [32]byte
	overflow[31] = 0xff
	c.Put(overflow, true)
	require.Equal(t, depositSignatureCacheCap, c.Len())
	require.Equal(t, false, c.Has(overflow))

	// The earliest entry must survive: a warmed result is never displaced.
	require.Equal(t, true, c.Has([32]byte{}))

	// Updating an existing key is still allowed at capacity.
	c.Put([32]byte{}, false)
	valid, ok := c.Get([32]byte{})
	require.Equal(t, true, ok)
	require.Equal(t, false, valid)
	require.Equal(t, depositSignatureCacheCap, c.Len())
}

func TestDepositSignatureCache_Concurrent(t *testing.T) {
	c := newDepositSignatureCache()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := range 128 {
				var key [32]byte
				key[0] = byte(base)
				key[1] = byte(j)
				c.Put(key, j%2 == 0)
				c.Get(key)
				c.Has(key)
				c.Len()
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, 8*128, c.Len())
}

func TestDepositSignatureCache_Full(t *testing.T) {
	c := newDepositSignatureCache()
	require.Equal(t, false, c.Full())
	for i := range depositSignatureCacheCap {
		var key [32]byte
		key[0], key[1], key[2] = byte(i), byte(i>>8), byte(i>>16)
		c.Put(key, true)
	}
	require.Equal(t, true, c.Full())
	c.Clear()
	require.Equal(t, false, c.Full())
}
