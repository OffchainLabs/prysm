package proof_test

import (
	"encoding/binary"
	"encoding/hex"
	"testing"

	sszquery "github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	proof "github.com/OffchainLabs/prysm/v6/encoding/ssz/query/proof"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	ssz "github.com/prysmaticlabs/fastssz"
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

func TestHashTreeRootFromBytes_ContainerBasicTypeFields_VoluntaryExit(t *testing.T) {
	voluntaryExit := &ethpb.VoluntaryExit{
		Epoch:          12345,
		ValidatorIndex: 67890,
	}

	info, err := sszquery.AnalyzeObject(voluntaryExit)
	require.NoError(t, err)

	data, err := ssz.MarshalSSZ(voluntaryExit)
	require.NoError(t, err)

	root, err := proof.HashTreeRootFromBytes(info, data)
	require.NoError(t, err)

	expected, err := voluntaryExit.HashTreeRoot()
	require.NoError(t, err)

	assert.Equal(t, expected, root)
}

func TestHashTreeRootFromBytes_Container(t *testing.T) {
	// BeaconBlockHeader fields are fixed-size; the three roots are Bytes32.
	parentRoot := make([]byte, 32)
	stateRoot := make([]byte, 32)
	bodyRoot := make([]byte, 32)
	copy(parentRoot, []byte{0x01, 0x02, 0x03})
	copy(stateRoot, []byte{0x04, 0x05, 0x06})
	copy(bodyRoot, []byte{0x07, 0x08, 0x09})

	beaconBlockHeader := &ethpb.BeaconBlockHeader{
		Slot:          12345,
		ProposerIndex: 67890,
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		BodyRoot:      bodyRoot,
	}

	info, err := sszquery.AnalyzeObject(beaconBlockHeader)
	require.NoError(t, err)

	data, err := ssz.MarshalSSZ(beaconBlockHeader)
	require.NoError(t, err)

	hexData := hex.EncodeToString(data)
	t.Logf("SSZ data: %s", hexData)

	root, err := proof.HashTreeRootFromBytes(info, data)
	require.NoError(t, err)
	t.Logf("HashTreeRoot: %x", root[:])

	expected, err := beaconBlockHeader.HashTreeRoot()
	t.Logf("Expected:     %x", expected[:])
	require.NoError(t, err)

	assert.Equal(t, expected, root)
}
