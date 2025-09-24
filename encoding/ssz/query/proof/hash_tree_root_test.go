package proof_test

import (
	"encoding/binary"
	"encoding/hex"
	"testing"

	sszquery "github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	proof "github.com/OffchainLabs/prysm/v6/encoding/ssz/query/proof"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	sszquerypb "github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ssz "github.com/prysmaticlabs/fastssz"
)

func TestHashTreeRoot_Basic(t *testing.T) {
	// --- uint64 ---
	u64Info, err := sszquery.AnalyzeObject(new(uint64))
	require.NoError(t, err)

	// uint64(1) in little-endian
	u64 := make([]byte, 8)
	binary.LittleEndian.PutUint64(u64, 1)

	root, err := proof.HashTreeRoot(u64Info, u64)
	require.NoError(t, err)

	root, err = proof.HashTreeRoot(u64Info, u64)
	require.NoError(t, err)

	t.Logf("HashTreeRoot: %x", root[:])

	var expected [32]byte
	copy(expected[:], u64)
	assert.Equal(t, expected, root)

	// --- bool true ---
	boolInfo, err := sszquery.AnalyzeObject(new(bool))
	require.NoError(t, err)

	bTrue := []byte{0x01}
	root, err = proof.HashTreeRoot(boolInfo, bTrue)
	require.NoError(t, err)

	expected = [32]byte{0x01}
	assert.Equal(t, expected, root)

	// --- bool false ---
	bFalse := []byte{0x00}
	root, err = proof.HashTreeRoot(boolInfo, bFalse)
	require.NoError(t, err)

	expected = [32]byte{0x00}
	assert.Equal(t, expected, root)

	// --- byte (uint8) ---
	byteInfo, err := sszquery.AnalyzeObject(new(uint8))
	require.NoError(t, err)

	b := []byte{0xAB}
	root, err = proof.HashTreeRoot(byteInfo, b)
	require.NoError(t, err)

	expected = [32]byte{0xAB}
	assert.Equal(t, expected, root)
}

func TestHashTreeRoot_ContainerBasicTypeFields_VoluntaryExit(t *testing.T) {
	voluntaryExit := &ethpb.VoluntaryExit{
		Epoch:          12345,
		ValidatorIndex: 67890,
	}

	info, err := sszquery.AnalyzeObject(voluntaryExit)
	require.NoError(t, err)

	data, err := ssz.MarshalSSZ(voluntaryExit)
	require.NoError(t, err)

	root, err := proof.HashTreeRoot(info, data)
	require.NoError(t, err)

	expected, err := voluntaryExit.HashTreeRoot()
	require.NoError(t, err)

	assert.Equal(t, expected, root)
}

func TestHashTreeRoot_Container(t *testing.T) {
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

	root, err := proof.HashTreeRoot(info, data)
	require.NoError(t, err)
	t.Logf("HashTreeRoot: %x", root[:])

	expected, err := beaconBlockHeader.HashTreeRoot()
	t.Logf("Expected:     %x", expected[:])
	require.NoError(t, err)

	assert.Equal(t, expected, root)
}

func TestHashTreeRoot_Container_IndexedAttestationElectra(t *testing.T) {
	// Construct IndexedAttestationElectra with dummy data.
	dummyRoot, err := hexutil.Decode("0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2")
	require.NoError(t, err)
	dummySignature, err := hexutil.Decode("0xc3a2f7d9e4a1b0c8d5e6f1a0b3c7d0e9f8a7b6c5d4e3f2a1b9c8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1a0b9c8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e28b7c6d5e4f3a2b1c0d9e8f7a6b5c")
	require.NoError(t, err)
	expectedTargetRoot, err := hexutil.Decode("0x4242424242424242424242424242424242424242424242424242424242424242")
	require.NoError(t, err)

	/*
		0x
		e4000000
		04000000000000000500000000000000cf8e0d4e9587369b230
		1d0790347320302cc0943d5a1884560367e8208d920f207000000000000
		00cf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e820
		8d920f20900000000000000424242424242424242424242424242424242
		4242424242424242424242424242c3a2f7d9e4a1b0c8d5e6f1a0b3c7d0e
		9f8a7b6c5d4e3f2a1b9c8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9
		e8f7a6b5c4d3e2f1a0b9c8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d
		9e8f7a6b5c4d3e28b7c6d5e4f3a2b1c0d9e8f7a6b5c0100000000000000
		02000000000000000300000000000000
		010000000000000002000000000000000300000000000000

	*/
	/*
		slot: '4'
		index: '5'
		beacon_block_root: '0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2'
		source:
		epoch: '7'
		root: '0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2'
		target:
		epoch: '9'
		root: '0x4242424242424242424242424242424242424242424242424242424242424242'

		hash tree root: 0x1200b9222588e8d42cd1710575a9df240beefd3f6a036e6df122ecf71cedf675
		serialization: 0x04000000000000000500000000000000cf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f20700000000000000cf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f209000000000000004242424242424242424242424242424242424242424242424242424242424242
	*/
	indexedAtt := &ethpb.IndexedAttestationElectra{
		AttestingIndices: []uint64{1, 2, 3},
		Data: &ethpb.AttestationData{
			Slot:            4,
			CommitteeIndex:  5,
			BeaconBlockRoot: dummyRoot,
			Source: &ethpb.Checkpoint{
				Epoch: 7,
				Root:  dummyRoot,
			},
			Target: &ethpb.Checkpoint{
				Epoch: 9,
				Root:  expectedTargetRoot,
			},
		},
		Signature: dummySignature,
	}
	marshalledIndexedAtt, err := indexedAtt.MarshalSSZ()
	require.NoError(t, err)

	// Start with a pointer to empty object and calculate SSZ info of `IndexedAttestationElectra`.
	info, err := sszquery.AnalyzeObject(indexedAtt)
	require.NoError(t, err)
	assert.NotNil(t, info, "Expected non-nil SSZ info")

	hashTreeRoot, err := proof.HashTreeRoot(info, marshalledIndexedAtt)
	require.NoError(t, err)

	expectedHashTreeRoot, err := indexedAtt.HashTreeRoot()
	require.NoError(t, err)

	assert.Equal(t, expectedHashTreeRoot, hashTreeRoot, "Hash tree roots should match")
}

func TestHashTreeRoot_ListOfContainers(t *testing.T) {
	// Validators
	// [
	// 	{
	// 		pubkey: "0x123400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" ,
	// 		withdrawal_credentials: "0x5678000000000000000000000000000000000000000000000000000000000000",
	// 		effective_balance: "9",
	// 		slashed: false,
	// 		activation_eligibility_epoch: "11",
	// 		activation_epoch: "12",
	// 		exit_epoch: "13",
	// 		withdrawable_epoch: "14"
	// 	},
	// 	{
	// 		pubkey: "0x151600000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
	// 		withdrawal_credentials: "0x1718000000000000000000000000000000000000000000000000000000000000",
	// 		effective_balance: "19",
	// 		slashed: true,
	// 		activation_eligibility_epoch: "21",
	// 		activation_epoch: "22",
	// 		exit_epoch: "23",
	// 		withdrawable_epoch: "24"
	// 	}
	// ]
	// SSZ serialization: 0x12340000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000056780000000000000000000000000000000000000000000000000000000000000900000000000000000b000000000000000c000000000000000d000000000000000e0000000000000015160000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000017180000000000000000000000000000000000000000000000000000000000001300000000000000011500000000000000160000000000000017000000000000001800000000000000
	// Hash Tree Root: 0x962288f21c75709e133fdb585e1094fd434155702111adf3bc949e2f18f556d0
	// validators := &ValidatorList{
	// 	Validators: []*ethpb.Validator{
	// 		{
	// 			PublicKey: func() []byte {
	// 				// 48 bytes (96 hex chars)
	// 				b, _ := hexutil.Decode("0x123400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
	// 				return b
	// 			}(),
	// 			WithdrawalCredentials: func() []byte {
	// 				// 32 bytes (64 hex chars)
	// 				b, _ := hexutil.Decode("0x5678000000000000000000000000000000000000000000000000000000000000")
	// 				return b
	// 			}(),
	// 			EffectiveBalance:           9,
	// 			Slashed:                    false,
	// 			ActivationEligibilityEpoch: 11,
	// 			ActivationEpoch:            12,
	// 			ExitEpoch:                  13,
	// 			WithdrawableEpoch:          14,
	// 		},
	// 		{
	// 			PublicKey: func() []byte {
	// 				// 48 bytes (96 hex chars) — la anterior cadena estaba incompleta
	// 				b, _ := hexutil.Decode("0x151600000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
	// 				return b
	// 			}(),
	// 			WithdrawalCredentials: func() []byte {
	// 				// 32 bytes
	// 				b, _ := hexutil.Decode("0x1718000000000000000000000000000000000000000000000000000000000000")
	// 				return b
	// 			}(),
	// 			EffectiveBalance:           19,
	// 			Slashed:                    true,
	// 			ActivationEligibilityEpoch: 21,
	// 			ActivationEpoch:            22,
	// 			ExitEpoch:                  23,
	// 			WithdrawableEpoch:          24,
	// 		},
	// 	},
	// }

	// info, err := sszquery.AnalyzeObject(validators)
	// require.NoError(t, err)
	// assert.NotNil(t, info, "Expected non-nil SSZ info")

	// data := []byte("0x12340000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000056780000000000000000000000000000000000000000000000000000000000000900000000000000000b000000000000000c000000000000000d000000000000000e0000000000000015160000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000017180000000000000000000000000000000000000000000000000000000000001300000000000000011500000000000000160000000000000017000000000000001800000000000000")
	// t.Logf("SSZ data: %x", data)

	// root, err := proof.HashTreeRoot(info, data)
	// require.NoError(t, err)
	// t.Logf("HashTreeRoot: %x", root[:])

	// expected := []byte("0x962288f21c75709e133fdb585e1094fd434155702111adf3bc949e2f18f556d0")
	// t.Logf("Expected:     %x", expected)

	// assert.Equal(t, expected, root)
}

func TestHashTreeRoot_CustomTypes_FixedNestedContainer(t *testing.T) {
	info, err := sszquery.AnalyzeObject(new(sszquerypb.FixedNestedContainer))
	require.NoError(t, err)
	assert.NotNil(t, info, "Expected non-nil SSZ info")

	nested := &sszquerypb.FixedNestedContainer{
		Value1: 42,
		Value2: func() []byte {
			b, _ := hexutil.Decode("0x4242424242424242424242424242424242424242424242424242424242424242")
			return b
		}(),
	}
	serializedData, err := ssz.MarshalSSZ(nested)
	require.NoError(t, err)

	hashTreeRoot, err := proof.HashTreeRoot(info, serializedData)
	require.NoError(t, err)

	expectedHashTreeRoot, err := nested.HashTreeRoot()
	require.NoError(t, err)
	assert.Equal(t, expectedHashTreeRoot, hashTreeRoot)

}

func TestHashTreeRoot_CustomTypes_FixedTestContainer(t *testing.T) {

	testContainer := &sszquerypb.FixedTestContainer{
		FieldUint32: 123,
		FieldUint64: 123,
		FieldBool:   true,
		Nested: &sszquerypb.FixedNestedContainer{
			Value1: 42,
			Value2: func() []byte {
				b, _ := hexutil.Decode("0x4242424242424242424242424242424242424242424242424242424242424242")
				return b
			}(),
		},
		VectorField: []uint64{1, 2, 3, 4},
		TwoDimensionBytesField: [][]byte{
			func() []byte {
				b, _ := hexutil.Decode("0x0101010101010101010101010101010101010101010101010101010101010101")
				return b
			}(),
			func() []byte {
				b, _ := hexutil.Decode("0x0202020202020202020202020202020202020202020202020202020202020202")
				return b
			}(),
		},
		TrailingField: []byte("hello"),
	}

	info, err := sszquery.AnalyzeObject(testContainer)
	require.NoError(t, err)
	assert.NotNil(t, info, "Expected non-nil SSZ info")

	serializedData, err := ssz.MarshalSSZ(testContainer)
	require.NoError(t, err)
	hashTreeRoot, err := proof.HashTreeRoot(info, serializedData)
	require.NoError(t, err)

	expectedHashTreeRoot, err := testContainer.HashTreeRoot()
	require.NoError(t, err)
	assert.Equal(t, expectedHashTreeRoot, hashTreeRoot)
}

func TestHashTreeRoot_CustomTypes_BitlistContainer(t *testing.T) {
	tests := []struct {
		name        string
		bitlist     []byte
		description string
	}{
		// { NOTE: THIS TEST CASE MUST BE REVIEWED
		// 	name:        "empty_bitlist",
		// 	bitlist:     []byte{},
		// 	description: "Empty bitlist - should hash correctly with zero length",
		// },
		{
			name:        "small_bitlist",
			bitlist:     []byte{0b10101010, 0x01}, // 8 bits: 10101010 + length-delimiting bit
			description: "Small bitlist with 8 bits",
		},
		{
			name:        "medium_bitlist",
			bitlist:     []byte{0b11110000, 0b00001111, 0b10101010, 0x01}, // 24 bits + length-delimiting bit
			description: "Medium bitlist with 24 bits",
		},
		{
			name: "large_bitlist",
			bitlist: func() []byte {
				// Create a bitlist with 100 bits + length-delimiting bit
				bits := make([]byte, 13) // 100 bits = 12.5 bytes, so 13 bytes
				for i := 0; i < 12; i++ {
					bits[i] = 0b10101010 // Alternating pattern
				}
				bits[12] = 0b00010000 // 4 bits + length-delimiting bit (bit 4)
				return bits
			}(),
			description: "Large bitlist with 100 bits",
		},
		{
			name: "near_max_bitlist",
			bitlist: func() []byte {
				// Create a bitlist with 2000 bits (near the 2048 limit)
				numBytes := (2000 + 7) / 8       // 250 bytes for 2000 bits
				bits := make([]byte, numBytes+1) // +1 for length-delimiting bit
				for i := 0; i < numBytes; i++ {
					bits[i] = 0b11001100 // Pattern
				}
				// Set the length-delimiting bit at position 2000 % 8 = 0 in the last byte
				bits[numBytes] = 0b00000001 // Length-delimiting bit at position 0
				return bits
			}(),
			description: "Near maximum bitlist with 2000 bits (close to 2048 limit)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bitlistContainer := &sszquerypb.BitlistContainer{
				BitlistField: tt.bitlist,
			}

			info, err := sszquery.AnalyzeObject(bitlistContainer)
			require.NoError(t, err, "Failed to analyze BitlistContainer for test: %s", tt.description)
			assert.NotNil(t, info, "Expected non-nil SSZ info for test: %s", tt.description)

			serializedData, err := ssz.MarshalSSZ(bitlistContainer)
			t.Logf("Serialized data for test %s: %x", tt.name, serializedData)
			require.NoError(t, err, "Failed to marshal BitlistContainer for test: %s", tt.description)

			expectedHashTreeRoot, err := bitlistContainer.HashTreeRoot()
			require.NoError(t, err, "Failed to compute expected HashTreeRoot for test: %s", tt.description)

			hashTreeRoot, err := proof.HashTreeRoot(info, serializedData)
			require.NoError(t, err, "Failed to compute HashTreeRoot for test: %s", tt.description)

			assert.Equal(t, expectedHashTreeRoot, hashTreeRoot,
				"Hash tree roots should match for test: %s (bitlist length: %d bytes)",
				tt.description, len(tt.bitlist))

			t.Logf("Test %s passed - Bitlist length: %d bytes, Root: %x",
				tt.name, len(tt.bitlist), hashTreeRoot)
		})
	}
}

func TestHashTreeRoot_CustomTypes_BitvectorContainer(t *testing.T) {
	bitvector := []byte{0b10101010, 0b10101010, 0b10101010, 0b10101010}
	bitvectorContainer := &sszquerypb.BitvectorContainer{
		BitvectorField: bitvector,
	}

	info, err := sszquery.AnalyzeObject(bitvectorContainer)
	require.NoError(t, err)
	assert.NotNil(t, info, "Expected non-nil SSZ info")

	serializedData, err := ssz.MarshalSSZ(bitvectorContainer)
	require.NoError(t, err)

	hashTreeRoot, err := proof.HashTreeRoot(info, serializedData)
	require.NoError(t, err)

	expectedHashTreeRoot, err := bitvectorContainer.HashTreeRoot()
	require.NoError(t, err)
	assert.Equal(t, expectedHashTreeRoot, hashTreeRoot)
}

func TestHashTreeRoot_CustomTypes_BasicTypeListContainer(t *testing.T) {
	listField := []uint64{0, 1, 2, 3, 4, 0, 1, 2, 3, 4}
	basicTypeList := &sszquerypb.BasicTypeList{
		FieldListUint64: listField,
	}

	info, err := sszquery.AnalyzeObject(basicTypeList)
	require.NoError(t, err)
	assert.NotNil(t, info, "Expected non-nil SSZ info")
	serializedData, err := ssz.MarshalSSZ(basicTypeList)
	require.NoError(t, err)

	t.Logf("Serialized data: %x", serializedData)

	expectedHashTreeRoot, err := basicTypeList.HashTreeRoot()
	require.NoError(t, err)

	hashTreeRoot, err := proof.HashTreeRoot(info, serializedData)
	require.NoError(t, err)

	t.Logf("Computed HashTreeRoot: %x\n", hashTreeRoot)
	t.Logf("Expected HashTreeRoot: %x\n", expectedHashTreeRoot)

	assert.Equal(t, expectedHashTreeRoot, hashTreeRoot)
}
