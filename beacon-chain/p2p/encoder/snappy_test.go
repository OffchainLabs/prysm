package encoder

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/golang/snappy"
)

func TestSszNetworkEncoder_BufferedReader(t *testing.T) {
	r := make([]byte, 10)
	bufR := snappy.NewReader(bytes.NewBuffer(r))
	ptr := reflect.ValueOf(bufR).Pointer()
	bufReaderPool.Put(bufR)

	r2 := make([]byte, 10)
	rdr := newBufferedReader(bytes.NewBuffer(r2))

	nPtr := reflect.ValueOf(rdr).Pointer()
	assert.Equal(t, ptr, nPtr, "invalid pointer value")
}

func TestSszNetworkEncoder_BufferedWriter(t *testing.T) {
	r := make([]byte, 10)
	bufR := snappy.NewBufferedWriter(bytes.NewBuffer(r))
	ptr := reflect.ValueOf(bufR).Pointer()
	bufWriterPool.Put(bufR)

	r2 := make([]byte, 10)
	rdr := newBufferedWriter(bytes.NewBuffer(r2))

	nPtr := reflect.ValueOf(rdr).Pointer()
	assert.Equal(t, ptr, nPtr, "invalid pointer value")
}

func TestDecodeSnappy(t *testing.T) {
	const maxSize = 10 * 1024 * 1024

	t.Run("bare header claiming max size", func(t *testing.T) {
		// Varint header for 10 MiB, with no body.
		msg := []byte{0x80, 0x80, 0x80, 0x05}
		_, err := DecodeSnappy(msg, maxSize)
		require.ErrorContains(t, "snappy message claims more data than it can hold", err)
	})

	t.Run("claimed length above max size", func(t *testing.T) {
		msg := []byte{0x81, 0x80, 0x80, 0x05}
		_, err := DecodeSnappy(msg, maxSize)
		require.ErrorContains(t, "snappy message exceeds max size", err)
	})

	t.Run("claimed length above expansion bound", func(t *testing.T) {
		// Keep a real snappy body but forge a header claiming more than the body can expand to.
		msg := snappy.Encode(nil, make([]byte, 1024))
		_, headerLen := binary.Uvarint(msg)
		body := msg[headerLen:]

		claimed := maxSnappyDecodedLen(len(body)+binary.MaxVarintLen64) + 1
		forged := binary.AppendUvarint(nil, claimed)
		forged = append(forged, body...)

		_, err := DecodeSnappy(forged, maxSize)
		require.ErrorContains(t, "snappy message claims more data than it can hold", err)
	})

	t.Run("highly compressible max size payload", func(t *testing.T) {
		payload := make([]byte, maxSize)
		msg := snappy.Encode(nil, payload)
		decoded, err := DecodeSnappy(msg, maxSize)
		require.NoError(t, err)
		require.DeepEqual(t, payload, decoded)
	})

	t.Run("regular payload", func(t *testing.T) {
		payload := []byte("the quick brown fox jumps over the lazy dog")
		msg := snappy.Encode(nil, payload)
		decoded, err := DecodeSnappy(msg, maxSize)
		require.NoError(t, err)
		require.DeepEqual(t, payload, decoded)
	})

	t.Run("empty payload", func(t *testing.T) {
		msg := snappy.Encode(nil, nil)
		decoded, err := DecodeSnappy(msg, maxSize)
		require.NoError(t, err)
		require.Equal(t, 0, len(decoded))
	})
}
