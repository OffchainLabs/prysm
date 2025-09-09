package proof_test

import (
	"encoding/binary"
	"testing"

	sszquery "github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	proof "github.com/OffchainLabs/prysm/v6/encoding/ssz/query/proof"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestHashTreeRootFromBytes_Basic(t *testing.T) {
	// --- uint64 ---
	u64Info, err := sszquery.AnalyzeObject(new(uint64))
	require.NoError(t, err)

	// uint64(1) in little-endian
	u64 := make([]byte, 8)
	binary.LittleEndian.PutUint64(u64, 1)

	root, err := proof.HashTreeRootFromBytes(u64Info, u64)
	require.NoError(t, err)

	var expected [32]byte
	copy(expected[:], u64)
	assert.Equal(t, expected, root)

	// --- bool true ---
	boolInfo, err := sszquery.AnalyzeObject(new(bool))
	require.NoError(t, err)

	bTrue := []byte{0x01}
	root, err = proof.HashTreeRootFromBytes(boolInfo, bTrue)
	require.NoError(t, err)

	expected = [32]byte{0x01}
	assert.Equal(t, expected, root)

	// --- bool false ---
	bFalse := []byte{0x00}
	root, err = proof.HashTreeRootFromBytes(boolInfo, bFalse)
	require.NoError(t, err)

	expected = [32]byte{0x00}
	assert.Equal(t, expected, root)

	// --- byte (uint8) ---
	byteInfo, err := sszquery.AnalyzeObject(new(uint8))
	require.NoError(t, err)

	b := []byte{0xAB}
	root, err = proof.HashTreeRootFromBytes(byteInfo, b)
	require.NoError(t, err)

	expected = [32]byte{0xAB}
	assert.Equal(t, expected, root)
}
