package fieldtrie

import (
	"encoding/binary"
	"fmt"
	"testing"

	customtypes "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/custom-types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	multi_value_slice "github.com/OffchainLabs/prysm/v7/container/multi-value-slice"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	mvslice "github.com/OffchainLabs/prysm/v7/container/multi-value-slice"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func Test_handlePendingAttestation_OutOfRange(t *testing.T) {
	items := make([]*ethpb.PendingAttestation, 1)
	indices := []uint64{3}
	_, err := handlePendingAttestationSlice(items, indices)
	assert.ErrorContains(t, "index 3 greater than number of pending attestations 1", err)
}

func Test_handleEth1DataSlice_OutOfRange(t *testing.T) {
	items := make([]*ethpb.Eth1Data, 1)
	indices := []uint64{3}
	_, err := handleEth1DataSlice(items, indices)
	assert.ErrorContains(t, "index 3 greater than number of items in eth1 data slice 1", err)

}

func Test_handleValidatorSlice_OutOfRange(t *testing.T) {
	vals := make([]stateutil.CompactValidator, 1)
	indices := []uint64{3}
	_, err := handleValidatorMVSlice(mvslice.BuildEmptyCompositeSlice(vals), indices)
	assert.ErrorContains(t, "index 3 greater than number of validators 1", err)
}

func TestBalancesSlice_CorrectRoots_All(t *testing.T) {
	balances := []uint64{5, 2929, 34, 1291, 354305}
	roots, err := handleBalanceMVSlice(mvslice.BuildEmptyCompositeSlice(balances), nil)
	assert.NoError(t, err)

	var root1 [32]byte
	binary.LittleEndian.PutUint64(root1[:8], balances[0])
	binary.LittleEndian.PutUint64(root1[8:16], balances[1])
	binary.LittleEndian.PutUint64(root1[16:24], balances[2])
	binary.LittleEndian.PutUint64(root1[24:32], balances[3])

	var root2 [32]byte
	binary.LittleEndian.PutUint64(root2[:8], balances[4])

	assert.DeepEqual(t, roots, [][32]byte{root1, root2})
}

func TestBalancesSlice_CorrectRoots_Some(t *testing.T) {
	balances := []uint64{5, 2929, 34, 1291, 354305}
	// Indices are chunk-level: chunk 0 contains balances[0..3], chunk 1 contains balances[4].
	roots, err := handleBalanceMVSlice(mvslice.BuildEmptyCompositeSlice(balances), []uint64{0, 1})
	assert.NoError(t, err)

	var chunk0 [32]byte
	binary.LittleEndian.PutUint64(chunk0[:8], balances[0])
	binary.LittleEndian.PutUint64(chunk0[8:16], balances[1])
	binary.LittleEndian.PutUint64(chunk0[16:24], balances[2])
	binary.LittleEndian.PutUint64(chunk0[24:32], balances[3])

	var chunk1 [32]byte
	binary.LittleEndian.PutUint64(chunk1[:8], balances[4])

	assert.DeepEqual(t, roots, [][32]byte{chunk0, chunk1})
}

func TestValidateIndices_CompressedField(t *testing.T) {
	fakeTrie := &FieldTrie{
		field:      types.Balances,
		dataType:   types.CompressedArray,
		length:     params.BeaconConfig().ValidatorRegistryLimit / 4,
		numOfElems: 0,
	}
	goodIdx := params.BeaconConfig().ValidatorRegistryLimit - 1
	assert.NoError(t, fakeTrie.validateIndices([]uint64{goodIdx}))

	badIdx := goodIdx + 1
	assert.ErrorContains(t, "invalid index for field balances", fakeTrie.validateIndices([]uint64{badIdx}))

}

func TestGrowFlatBuffer_ZeroHashInitialization(t *testing.T) {
	// Build a trie with 4 leaves, then grow to 8 and set leaf 4.
	// This tests that new upper-level entries are initialized to ZeroHashes[level],
	// not [32]byte{}. Without correct initialization, the neighbor at level 1
	// index 3 would be read as [32]byte{} instead of ZeroHashes[1], producing
	// an incorrect root.
	depth := uint64(3)
	leaves := [][32]byte{
		{1}, {2}, {3}, {4},
	}
	offsets := computeOffsets(depth, uint64(len(leaves)))
	nodes := make([][32]byte, offsets[depth+1])
	copy(nodes, leaves)
	hashUpFromLeaves(nodes, offsets)

	// Compute the expected root by building a full 8-leaf trie from scratch
	// with the 5th leaf set and leaves 5-7 as zero.
	expectedLeaves := make([][32]byte, 8)
	copy(expectedLeaves, leaves)
	expectedLeaves[4] = [32]byte{5}
	expectedOffsets := computeOffsets(depth, 8)
	expectedNodes := make([][32]byte, expectedOffsets[depth+1])
	copy(expectedNodes, expectedLeaves)
	hashUpFromLeaves(expectedNodes, expectedOffsets)
	expectedRoot := expectedNodes[expectedOffsets[depth]]

	// Grow the original trie from 4 to 8 leaves in one step, then
	// set leaf 4 and rehash all upper levels.
	ft := &FieldTrie{}
	ft.InsertFieldLayer(nodes, offsets)
	ft.ensureLeafCapacity(8)
	ft.nodesData.nodes[4] = [32]byte{5}
	hashUpFromLeaves(ft.nodesData.nodes, ft.nodesData.offsets)
	root := ft.nodesData.nodes[ft.nodesData.offsets[depth]]

	assert.Equal(t, expectedRoot, root,
		"Root mismatch: ensureLeafCapacity must initialize new upper-level entries to ZeroHashes[level]")
}

func TestFieldTrie_NativeState_fieldConvertersNative(t *testing.T) {
	type args struct {
		field    types.FieldIndex
		indices  []uint64
		elements any
	}
	tests := []struct {
		name           string
		args           *args
		wantHex        []string
		errMsg         string
		expectedLength int
	}{
		{
			name: "BlockRoots customtypes.BlockRoots",
			args: &args{
				field:    types.FieldIndex(5),
				elements: customtypes.BlockRoots{},
			},
			wantHex:        []string{"0x0000000000000000000000000000000000000000000000000000000000000000"},
			expectedLength: 8192,
		},
		{
			name: "BlockRoots type not found",
			args: &args{
				field:    types.FieldIndex(5),
				elements: 123,
			},
			wantHex: nil,
			errMsg:  "non-existent type provided",
		},
		{
			name: "StateRoots customtypes.StateRoots",
			args: &args{
				field:    types.FieldIndex(6),
				elements: customtypes.StateRoots{},
			},
			wantHex:        []string{"0x0000000000000000000000000000000000000000000000000000000000000000"},
			expectedLength: 8192,
		},
		{
			name: "StateRoots type not found",
			args: &args{
				field:    types.FieldIndex(6),
				elements: 123,
			},
			wantHex: nil,
			errMsg:  "non-existent type provided",
		},
		{
			name: "StateRoots with empty indices",
			args: &args{
				field:    types.FieldIndex(6),
				elements: customtypes.StateRoots{},
			},
			wantHex:        []string{"0x0000000000000000000000000000000000000000000000000000000000000000"},
			expectedLength: 8192,
		},
		{
			name: "RandaoMixes customtypes.RandaoMixes",
			args: &args{
				field:    types.FieldIndex(13),
				elements: customtypes.RandaoMixes{},
			},
			wantHex:        []string{"0x0000000000000000000000000000000000000000000000000000000000000000"},
			expectedLength: 65536,
		},
		{
			name: "RandaoMixes type not found",
			args: &args{
				field:    types.FieldIndex(13),
				elements: 123,
			},
			wantHex: nil,
			errMsg:  "non-existent type provided",
		},
		{
			name: "Eth1DataVotes all",
			args: &args{
				field: types.FieldIndex(9),
				elements: []*ethpb.Eth1Data{
					{
						DepositRoot:  make([]byte, fieldparams.RootLength),
						DepositCount: 1,
					},
				},
			},
			wantHex: []string{"0x4833912e1264aef8a18392d795f3f2eed17cf5c0e8471cb0c0db2ec5aca10231"},
		},
		{
			name: "Eth1DataVotes by index",
			args: &args{
				field:   types.FieldIndex(9),
				indices: []uint64{1},
				elements: []*ethpb.Eth1Data{
					{
						DepositRoot:  make([]byte, fieldparams.RootLength),
						DepositCount: 2,
					},
					{
						DepositRoot:  make([]byte, fieldparams.RootLength),
						DepositCount: 1,
					},
				},
			},
			wantHex: []string{"0x4833912e1264aef8a18392d795f3f2eed17cf5c0e8471cb0c0db2ec5aca10231"},
		},
		{
			name: "Eth1DataVotes type not found",
			args: &args{
				field:    types.FieldIndex(9),
				elements: 123,
			},
			wantHex: nil,
			errMsg:  fmt.Sprintf("Wanted type of %T", []*ethpb.Eth1Data{}),
		},
		{
			name: "Balance",
			args: &args{
				field:    types.FieldIndex(12),
				elements: []uint64{12321312321, 12131241234123123},
			},
			wantHex: []string{"0x414e68de0200000073c971b44c192b0000000000000000000000000000000000"},
		},
		{
			name: "Validators",
			args: &args{
				field: types.FieldIndex(11),
				elements: []stateutil.CompactValidator{
					{
						ActivationEpoch: 1,
					},
				},
			},
			wantHex: []string{"0x79817c24fc7ba90cdac48fd462fafc1cb501884e847b18733f7ca6df214a301e"},
		},
		{
			name: "Validators not found",
			args: &args{
				field:    types.FieldIndex(11),
				elements: 123,
			},
			wantHex: nil,
			errMsg:  "Wanted type of CompactValidator",
		},
		{
			name: "Attestations all",
			args: &args{
				field: types.FieldIndex(15),
				elements: []*ethpb.PendingAttestation{
					{
						ProposerIndex: 1,
					},
				},
			},
			wantHex: []string{"0x7d7696e7f12593934afcd87a0d38e1a981bee63cb4cf0568ba36a6e0596eeccb"},
		},
		{
			name: "Attestations by index",
			args: &args{
				field:   types.FieldIndex(15),
				indices: []uint64{1},
				elements: []*ethpb.PendingAttestation{
					{
						ProposerIndex: 0,
					},
					{
						ProposerIndex: 1,
					},
				},
			},
			wantHex: []string{"0x7d7696e7f12593934afcd87a0d38e1a981bee63cb4cf0568ba36a6e0596eeccb"},
		},
		{
			name: "Type not found",
			args: &args{
				field: types.FieldIndex(999),
				elements: []*ethpb.PendingAttestation{
					{
						ProposerIndex: 1,
					},
				},
			},
			errMsg: "got unsupported type of",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots, err := fieldConverters(tt.args.field, tt.args.elements, tt.args.indices)
			if err != nil {
				if tt.errMsg != "" {
					require.ErrorContains(t, tt.errMsg, err)
				} else {
					t.Error("Unexpected error: " + err.Error())
				}
			} else {
				for i, root := range roots {
					hex := hexutil.Encode(root[:])
					require.Equal(t, tt.wantHex[i], hex)
					if tt.expectedLength != 0 {
						require.Equal(t, len(roots), tt.expectedLength)
						break
					}
				}
			}
		})
	}
}

func TestElemCount(t *testing.T) {
	t.Run("nil returns 0", func(t *testing.T) {
		require.Equal(t, uint64(0), elemCount(nil))
	})

	t.Run("sliceAccessor returns Len", func(t *testing.T) {
		require.Equal(t, uint64(42), elemCount(mockSliceAccessor{len: 42}))
	})

	t.Run("regular slice uses reflect", func(t *testing.T) {
		require.Equal(t, uint64(5), elemCount([]uint64{1, 2, 3, 4, 5}))
	})

	t.Run("BlockRoots uses reflect", func(t *testing.T) {
		require.Equal(t, uint64(3), elemCount(make(customtypes.BlockRoots, 3)))
	})
}

// mockSliceAccessor implements sliceAccessor for testing.
type mockSliceAccessor struct {
	len int
}

func (m mockSliceAccessor) Len(_ multi_value_slice.Identifiable) int { return m.len }
func (m mockSliceAccessor) State() multi_value_slice.Identifiable    { return nil }

// TestValidateElements verifies validateElements for nil elements and
// the sliceAccessor path (both within and exceeding the length limit).
func TestValidateElements(t *testing.T) {
	t.Run("nil elements returns nil", func(t *testing.T) {
		err := validateElements(types.BlockRoots, types.BasicArray, nil, testBlockRootsSize)
		require.NoError(t, err)
	})

	t.Run("sliceAccessor within length returns nil", func(t *testing.T) {
		err := validateElements(types.BlockRoots, types.BasicArray, mockSliceAccessor{len: 100}, testBlockRootsSize)
		require.NoError(t, err)
	})

	t.Run("sliceAccessor exceeding length returns error", func(t *testing.T) {
		err := validateElements(types.BlockRoots, types.BasicArray, mockSliceAccessor{len: testBlockRootsSize + 1}, testBlockRootsSize)
		require.ErrorContains(t, "elements length is larger than expected", err)
	})
}
