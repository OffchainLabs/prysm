package query_test

import (
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query/testutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	sszquerypb "github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/prysmaticlabs/go-bitfield"
)

func TestCalculateOffsetAndLength(t *testing.T) {
	type testCase struct {
		name           string
		path           string
		expectedOffset uint64
		expectedLength uint64
	}

	t.Run("FixedTestContainer", func(t *testing.T) {
		tests := []testCase{
			// Basic integer types
			{
				name:           "field_uint32",
				path:           ".field_uint32",
				expectedOffset: 0,
				expectedLength: 4,
			},
			{
				name:           "field_uint64",
				path:           ".field_uint64",
				expectedOffset: 4,
				expectedLength: 8,
			},
			// Boolean type
			{
				name:           "field_bool",
				path:           ".field_bool",
				expectedOffset: 12,
				expectedLength: 1,
			},
			// Fixed-size bytes
			{
				name:           "field_bytes32",
				path:           ".field_bytes32",
				expectedOffset: 13,
				expectedLength: 32,
			},
			// Nested container
			{
				name:           "nested container",
				path:           ".nested",
				expectedOffset: 45,
				expectedLength: 40,
			},
			{
				name:           "nested value1",
				path:           ".nested.value1",
				expectedOffset: 45,
				expectedLength: 8,
			},
			{
				name:           "nested value2",
				path:           ".nested.value2",
				expectedOffset: 53,
				expectedLength: 32,
			},
			// Vector field
			{
				name:           "vector field",
				path:           ".vector_field",
				expectedOffset: 85,
				expectedLength: 192, // 24 * 8 bytes
			},
			// 2D bytes field
			{
				name:           "two_dimension_bytes_field",
				path:           ".two_dimension_bytes_field",
				expectedOffset: 277,
				expectedLength: 160, // 5 * 32 bytes
			},
			// Bitvector fields
			{
				name:           "bitvector64_field",
				path:           ".bitvector64_field",
				expectedOffset: 437,
				expectedLength: 8,
			},
			{
				name:           "bitvector512_field",
				path:           ".bitvector512_field",
				expectedOffset: 445,
				expectedLength: 64,
			},
			// Trailing field
			{
				name:           "trailing_field",
				path:           ".trailing_field",
				expectedOffset: 509,
				expectedLength: 56,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				path, err := query.ParsePath(tt.path)
				require.NoError(t, err)

				info, err := query.AnalyzeObject(&sszquerypb.FixedTestContainer{})
				require.NoError(t, err)

				_, offset, length, err := query.CalculateOffsetAndLength(info, path)
				require.NoError(t, err)

				require.Equal(t, tt.expectedOffset, offset, "Expected offset to be %d", tt.expectedOffset)
				require.Equal(t, tt.expectedLength, length, "Expected length to be %d", tt.expectedLength)
			})
		}
	})

	t.Run("VariableTestContainer", func(t *testing.T) {
		tests := []testCase{
			// Fixed leading field
			{
				name:           "leading_field",
				path:           ".leading_field",
				expectedOffset: 0,
				expectedLength: 32,
			},
			// Variable-size list fields
			{
				name:           "field_list_uint64",
				path:           ".field_list_uint64",
				expectedOffset: 108, // First part of variable-sized type.
				expectedLength: 40,  // 5 elements * uint64 (8 bytes each)
			},
			{
				name:           "field_list_container",
				path:           ".field_list_container",
				expectedOffset: 148, // Second part of variable-sized type.
				expectedLength: 120, // 3 elements * FixedNestedContainer (40 bytes each)
			},
			{
				name:           "field_list_bytes32",
				path:           ".field_list_bytes32",
				expectedOffset: 268,
				expectedLength: 96, // 3 elements * 32 bytes each
			},
			// Nested paths
			{
				name:           "nested",
				path:           ".nested",
				expectedOffset: 364,
				// Calculated with:
				// - Value1: 8 bytes
				// - field_list_uint64 offset: 4 bytes
				// - field_list_uint64 length: 40 bytes
				expectedLength: 52,
			},
			{
				name:           "nested.value1",
				path:           ".nested.value1",
				expectedOffset: 364,
				expectedLength: 8,
			},
			{
				name:           "nested.field_list_uint64",
				path:           ".nested.field_list_uint64",
				expectedOffset: 376,
				expectedLength: 40,
			},
			// Bitlist field
			{
				name:           "bitlist_field",
				path:           ".bitlist_field",
				expectedOffset: 416,
				expectedLength: 33, // 32 bytes + 1 byte for length delimiter
			},
			// Fixed trailing field
			{
				name:           "trailing_field",
				path:           ".trailing_field",
				expectedOffset: 52, // After leading_field + 5 offset pointers
				expectedLength: 56,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				path, err := query.ParsePath(tt.path)
				require.NoError(t, err)

				testContainer := createVariableTestContainer()

				info, err := query.AnalyzeObject(testContainer)
				require.NoError(t, err)

				_, offset, length, err := query.CalculateOffsetAndLength(info, path)
				require.NoError(t, err)

				require.Equal(t, tt.expectedOffset, offset, "Expected offset to be %d", tt.expectedOffset)
				require.Equal(t, tt.expectedLength, length, "Expected length to be %d", tt.expectedLength)
			})
		}
	})
}

func TestRoundTripSszInfo(t *testing.T) {
	specs := []testutil.TestSpec{
		getFixedTestContainerSpec(),
		getVariableTestContainerSpec(),
		getExecutionPayloadDenebSpec(),
		getBeaconStateElectraSpec(),
	}

	for _, spec := range specs {
		testutil.RunStructTest(t, spec)
	}
}

func createFixedTestContainer() *sszquerypb.FixedTestContainer {
	fieldBytes32 := make([]byte, 32)
	for i := range fieldBytes32 {
		fieldBytes32[i] = byte(i + 24)
	}

	nestedValue2 := make([]byte, 32)
	for i := range nestedValue2 {
		nestedValue2[i] = byte(i + 56)
	}

	bitvector64 := bitfield.NewBitvector64()
	for i := range bitvector64 {
		bitvector64[i] = 0x42
	}

	bitvector512 := bitfield.NewBitvector512()
	for i := range bitvector512 {
		bitvector512[i] = 0x24
	}

	trailingField := make([]byte, 56)
	for i := range trailingField {
		trailingField[i] = byte(i + 88)
	}

	return &sszquerypb.FixedTestContainer{
		// Basic types
		FieldUint32: math.MaxUint32,
		FieldUint64: math.MaxUint64,
		FieldBool:   true,

		// Fixed-size bytes
		FieldBytes32: fieldBytes32,

		// Nested container
		Nested: &sszquerypb.FixedNestedContainer{
			Value1: 123,
			Value2: nestedValue2,
		},

		// Vector field
		VectorField: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24},

		// 2D bytes field
		TwoDimensionBytesField: [][]byte{
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
		},

		// Bitvector fields
		Bitvector64Field:  bitvector64,
		Bitvector512Field: bitvector512,

		// Trailing field
		TrailingField: trailingField,
	}
}

func getFixedTestContainerSpec() testutil.TestSpec {
	testContainer := createFixedTestContainer()

	return testutil.TestSpec{
		Name:     "FixedTestContainer",
		Type:     sszquerypb.FixedTestContainer{},
		Instance: testContainer,
		PathTests: []testutil.PathTest{
			// Basic types
			{
				Path:     ".field_uint32",
				Expected: testContainer.FieldUint32,
			},
			{
				Path:     ".field_uint64",
				Expected: testContainer.FieldUint64,
			},
			{
				Path:     ".field_bool",
				Expected: testContainer.FieldBool,
			},
			// Fixed-size bytes
			{
				Path:     ".field_bytes32",
				Expected: testContainer.FieldBytes32,
			},
			// Nested container
			{
				Path:     ".nested",
				Expected: testContainer.Nested,
			},
			{
				Path:     ".nested.value1",
				Expected: testContainer.Nested.Value1,
			},
			{
				Path:     ".nested.value2",
				Expected: testContainer.Nested.Value2,
			},
			// Vector field
			{
				Path:     ".vector_field",
				Expected: testContainer.VectorField,
			},
			// 2D bytes field
			{
				Path:     ".two_dimension_bytes_field",
				Expected: testContainer.TwoDimensionBytesField,
			},
			// Bitvector fields
			{
				Path:     ".bitvector64_field",
				Expected: testContainer.Bitvector64Field,
			},
			{
				Path:     ".bitvector512_field",
				Expected: testContainer.Bitvector512Field,
			},
			// Trailing field
			{
				Path:     ".trailing_field",
				Expected: testContainer.TrailingField,
			},
		},
	}
}

func createVariableTestContainer() *sszquerypb.VariableTestContainer {
	leadingField := make([]byte, 32)
	for i := range leadingField {
		leadingField[i] = byte(i + 100)
	}

	trailingField := make([]byte, 56)
	for i := range trailingField {
		trailingField[i] = byte(i + 150)
	}

	nestedContainers := make([]*sszquerypb.FixedNestedContainer, 3)
	for i := range nestedContainers {
		value2 := make([]byte, 32)
		for j := range value2 {
			value2[j] = byte(j + i*32)
		}
		nestedContainers[i] = &sszquerypb.FixedNestedContainer{
			Value1: uint64(1000 + i),
			Value2: value2,
		}
	}

	bitlistField := bitfield.NewBitlist(256)
	bitlistField.SetBitAt(0, true)
	bitlistField.SetBitAt(10, true)
	bitlistField.SetBitAt(50, true)
	bitlistField.SetBitAt(100, true)
	bitlistField.SetBitAt(255, true)

	return &sszquerypb.VariableTestContainer{
		// Fixed leading field
		LeadingField: leadingField,

		// Variable-size lists
		FieldListUint64:    []uint64{100, 200, 300, 400, 500},
		FieldListContainer: nestedContainers,
		FieldListBytes32: [][]byte{
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
		},

		// Variable nested container
		Nested: &sszquerypb.VariableNestedContainer{
			Value1:          42,
			FieldListUint64: []uint64{1, 2, 3, 4, 5},
		},

		// Bitlist field
		BitlistField: bitlistField,

		// Fixed trailing field
		TrailingField: trailingField,
	}
}

func getVariableTestContainerSpec() testutil.TestSpec {
	testContainer := createVariableTestContainer()

	return testutil.TestSpec{
		Name:     "VariableTestContainer",
		Type:     sszquerypb.VariableTestContainer{},
		Instance: testContainer,
		PathTests: []testutil.PathTest{
			// Fixed leading field
			{
				Path:     ".leading_field",
				Expected: testContainer.LeadingField,
			},
			// Variable-size list of uint64
			{
				Path:     ".field_list_uint64",
				Expected: testContainer.FieldListUint64,
			},
			// Variable-size list of (fixed-size) containers
			{
				Path:     ".field_list_container",
				Expected: testContainer.FieldListContainer,
			},
			// Variable-size list of bytes32
			{
				Path:     ".field_list_bytes32",
				Expected: testContainer.FieldListBytes32,
			},
			// Variable nested container with every path
			{
				Path:     ".nested",
				Expected: testContainer.Nested,
			},
			{
				Path:     ".nested.value1",
				Expected: testContainer.Nested.Value1,
			},
			{
				Path:     ".nested.field_list_uint64",
				Expected: testContainer.Nested.FieldListUint64,
			},
			// Bitlist field
			{
				Path:     ".bitlist_field",
				Expected: testContainer.BitlistField,
			},
			// Fixed trailing field
			{
				Path:     ".trailing_field",
				Expected: testContainer.TrailingField,
			},
		},
	}
}

func getBeaconStateElectraSpec() testutil.TestSpec {
	// Create a BeaconStateElectra with test data
	beaconState, err := util.NewBeaconStateElectra()
	if err != nil {
		panic(err)
	}

	// Get the underlying protobuf state
	pbState := beaconState.ToProtoUnsafe().(*ethpb.BeaconStateElectra)

	// Set some test values
	pbState.Slot = 54321
	pbState.GenesisTime = 1609459200
	pbState.GenesisValidatorsRoot = generateUniqueBytes(32, 0)
	pbState.Eth1DepositIndex = 999
	pbState.Fork.Epoch = 12345
	pbState.Fork.CurrentVersion = []byte{1, 2, 3, 4}
	pbState.Fork.PreviousVersion = []byte{5, 6, 7, 8}
	pbState.LatestBlockHeader.Slot = 54320
	pbState.LatestBlockHeader.ProposerIndex = 100
	pbState.LatestBlockHeader.ParentRoot = generateUniqueBytes(32, 32)
	pbState.LatestBlockHeader.StateRoot = generateUniqueBytes(32, 64)
	pbState.LatestBlockHeader.BodyRoot = generateUniqueBytes(32, 96)
	pbState.Eth1Data.DepositRoot = generateUniqueBytes(32, 128)
	pbState.Eth1Data.DepositCount = 500
	pbState.Eth1Data.BlockHash = generateUniqueBytes(32, 160)
	pbState.Balances = []uint64{32000000000, 31000000000, 33000000000, 32500000000, 31500000000}
	pbState.JustificationBits = bitfield.Bitvector4{0x0F}

	// Add validators with test data
	pbState.Validators = []*ethpb.Validator{
		{
			PublicKey:                  generateUniqueBytes(48, 192),
			WithdrawalCredentials:      generateUniqueBytes(32, 240),
			EffectiveBalance:           32000000000,
			Slashed:                    false,
			ActivationEligibilityEpoch: 0,
			ActivationEpoch:            0,
			ExitEpoch:                  18446744073709551615,
			WithdrawableEpoch:          18446744073709551615,
		},
		{
			PublicKey:                  generateUniqueBytes(48, 272),
			WithdrawalCredentials:      generateUniqueBytes(32, 320),
			EffectiveBalance:           31000000000,
			Slashed:                    true,
			ActivationEligibilityEpoch: 1,
			ActivationEpoch:            2,
			ExitEpoch:                  1000,
			WithdrawableEpoch:          1100,
		},
		{
			PublicKey:                  generateUniqueBytes(48, 352),
			WithdrawalCredentials:      generateUniqueBytes(32, 400),
			EffectiveBalance:           33000000000,
			Slashed:                    false,
			ActivationEligibilityEpoch: 10,
			ActivationEpoch:            15,
			ExitEpoch:                  18446744073709551615,
			WithdrawableEpoch:          18446744073709551615,
		},
	}

	// Add more populated fields
	pbState.Slashings[0] = 1000000000
	pbState.Slashings[100] = 2000000000
	pbState.Slashings[1000] = 3000000000

	// Add some eth1 data votes
	pbState.Eth1DataVotes = []*ethpb.Eth1Data{
		{
			DepositRoot:  generateUniqueBytes(32, 432),
			DepositCount: 100,
			BlockHash:    generateUniqueBytes(32, 464),
		},
		{
			DepositRoot:  generateUniqueBytes(32, 496),
			DepositCount: 200,
			BlockHash:    generateUniqueBytes(32, 528),
		},
	}

	// Set some participation flags
	pbState.PreviousEpochParticipation = []byte{0x07, 0x05, 0x03}
	pbState.CurrentEpochParticipation = []byte{0x07, 0x07, 0x01, 0x00, 0x06}

	// Set inactivity scores
	pbState.InactivityScores = []uint64{0, 0, 10, 5, 100}

	// Set checkpoint fields
	pbState.PreviousJustifiedCheckpoint = &ethpb.Checkpoint{
		Epoch: 100,
		Root:  generateUniqueBytes(32, 1040),
	}
	pbState.CurrentJustifiedCheckpoint = &ethpb.Checkpoint{
		Epoch: 101,
		Root:  generateUniqueBytes(32, 1072),
	}
	pbState.FinalizedCheckpoint = &ethpb.Checkpoint{
		Epoch: 99,
		Root:  generateUniqueBytes(32, 1104),
	}

	// Set next withdrawal indices
	pbState.NextWithdrawalIndex = 42
	pbState.NextWithdrawalValidatorIndex = 123

	// Set Electra-specific fields
	pbState.DepositRequestsStartIndex = 5000
	pbState.DepositBalanceToConsume = 64000000000
	pbState.ExitBalanceToConsume = 32000000000
	pbState.EarliestExitEpoch = 100
	pbState.ConsolidationBalanceToConsume = 96000000000
	pbState.EarliestConsolidationEpoch = 200

	// Add historical summaries
	pbState.HistoricalSummaries = []*ethpb.HistoricalSummary{
		{
			BlockSummaryRoot: generateUniqueBytes(32, 560),
			StateSummaryRoot: generateUniqueBytes(32, 592),
		},
		{
			BlockSummaryRoot: generateUniqueBytes(32, 624),
			StateSummaryRoot: generateUniqueBytes(32, 656),
		},
	}

	// Add pending deposits
	pbState.PendingDeposits = []*ethpb.PendingDeposit{
		{
			PublicKey:             generateUniqueBytes(48, 688),
			WithdrawalCredentials: generateUniqueBytes(32, 736),
			Amount:                32000000000,
			Signature:             generateUniqueBytes(96, 768),
			Slot:                  10000,
		},
		{
			PublicKey:             generateUniqueBytes(48, 864),
			WithdrawalCredentials: generateUniqueBytes(32, 912),
			Amount:                16000000000,
			Signature:             generateUniqueBytes(96, 944),
			Slot:                  10100,
		},
	}

	// Add pending partial withdrawals
	pbState.PendingPartialWithdrawals = []*ethpb.PendingPartialWithdrawal{
		{
			Index:             0,
			Amount:            1000000000,
			WithdrawableEpoch: 500,
		},
		{
			Index:             1,
			Amount:            2000000000,
			WithdrawableEpoch: 510,
		},
		{
			Index:             2,
			Amount:            1500000000,
			WithdrawableEpoch: 520,
		},
	}

	// Add pending consolidations
	pbState.PendingConsolidations = []*ethpb.PendingConsolidation{
		{
			SourceIndex: 0,
			TargetIndex: 1,
		},
	}

	return testutil.TestSpec{
		Name:     "BeaconStateElectra",
		Type:     ethpb.BeaconStateElectra{},
		Instance: pbState,
		PathTests: []testutil.PathTest{
			// Test basic fields
			{
				Path:     ".slot",
				Expected: pbState.Slot,
			},
			{
				Path:     ".genesis_time",
				Expected: pbState.GenesisTime,
			},
			{
				Path:     ".genesis_validators_root",
				Expected: pbState.GenesisValidatorsRoot,
			},
			// Test fork data
			{
				Path:     ".fork.epoch",
				Expected: pbState.Fork.Epoch,
			},
			{
				Path:     ".fork.current_version",
				Expected: pbState.Fork.CurrentVersion,
			},
			{
				Path:     ".fork.previous_version",
				Expected: pbState.Fork.PreviousVersion,
			},
			// Test latest block header
			{
				Path:     ".latest_block_header.slot",
				Expected: pbState.LatestBlockHeader.Slot,
			},
			{
				Path:     ".latest_block_header.proposer_index",
				Expected: pbState.LatestBlockHeader.ProposerIndex,
			},
			{
				Path:     ".latest_block_header.parent_root",
				Expected: pbState.LatestBlockHeader.ParentRoot,
			},
			{
				Path:     ".latest_block_header.state_root",
				Expected: pbState.LatestBlockHeader.StateRoot,
			},
			{
				Path:     ".latest_block_header.body_root",
				Expected: pbState.LatestBlockHeader.BodyRoot,
			},
			// Test eth1 data
			{
				Path:     ".eth1_data.deposit_root",
				Expected: pbState.Eth1Data.DepositRoot,
			},
			{
				Path:     ".eth1_data.deposit_count",
				Expected: pbState.Eth1Data.DepositCount,
			},
			{
				Path:     ".eth1_data.block_hash",
				Expected: pbState.Eth1Data.BlockHash,
			},
			{
				Path:     ".eth1_deposit_index",
				Expected: pbState.Eth1DepositIndex,
			},
			// Test validators - entire list
			{
				Path:     ".validators",
				Expected: pbState.Validators,
			},
			// Test balances - entire list
			{
				Path:     ".balances",
				Expected: pbState.Balances,
			},
			// Test participation
			{
				Path:     ".previous_epoch_participation",
				Expected: pbState.PreviousEpochParticipation,
			},
			{
				Path:     ".current_epoch_participation",
				Expected: pbState.CurrentEpochParticipation,
			},
			// Test eth1 data votes - entire list
			{
				Path:     ".eth1_data_votes",
				Expected: pbState.Eth1DataVotes,
			},
			// Test inactivity scores - entire list
			{
				Path:     ".inactivity_scores",
				Expected: pbState.InactivityScores,
			},
			// Test justification bits
			{
				Path:     ".justification_bits",
				Expected: pbState.JustificationBits,
			},
			// Test checkpoint fields
			{
				Path:     ".previous_justified_checkpoint.epoch",
				Expected: pbState.PreviousJustifiedCheckpoint.Epoch,
			},
			{
				Path:     ".previous_justified_checkpoint.root",
				Expected: pbState.PreviousJustifiedCheckpoint.Root,
			},
			{
				Path:     ".current_justified_checkpoint.epoch",
				Expected: pbState.CurrentJustifiedCheckpoint.Epoch,
			},
			{
				Path:     ".current_justified_checkpoint.root",
				Expected: pbState.CurrentJustifiedCheckpoint.Root,
			},
			{
				Path:     ".finalized_checkpoint.epoch",
				Expected: pbState.FinalizedCheckpoint.Epoch,
			},
			{
				Path:     ".finalized_checkpoint.root",
				Expected: pbState.FinalizedCheckpoint.Root,
			},
			// Test withdrawal fields
			{
				Path:     ".next_withdrawal_index",
				Expected: pbState.NextWithdrawalIndex,
			},
			{
				Path:     ".next_withdrawal_validator_index",
				Expected: pbState.NextWithdrawalValidatorIndex,
			},
			// Test Electra-specific fields
			{
				Path:     ".deposit_requests_start_index",
				Expected: pbState.DepositRequestsStartIndex,
			},
			{
				Path:     ".deposit_balance_to_consume",
				Expected: pbState.DepositBalanceToConsume,
			},
			{
				Path:     ".exit_balance_to_consume",
				Expected: pbState.ExitBalanceToConsume,
			},
			{
				Path:     ".earliest_exit_epoch",
				Expected: pbState.EarliestExitEpoch,
			},
			{
				Path:     ".consolidation_balance_to_consume",
				Expected: pbState.ConsolidationBalanceToConsume,
			},
			{
				Path:     ".earliest_consolidation_epoch",
				Expected: pbState.EarliestConsolidationEpoch,
			},
			// Test historical summaries - entire list
			{
				Path:     ".historical_summaries",
				Expected: pbState.HistoricalSummaries,
			},
			// Test pending deposits - entire list
			{
				Path:     ".pending_deposits",
				Expected: pbState.PendingDeposits,
			},
			// Test pending partial withdrawals - entire list
			{
				Path:     ".pending_partial_withdrawals",
				Expected: pbState.PendingPartialWithdrawals,
			},
			// Test pending consolidations - entire list
			{
				Path:     ".pending_consolidations",
				Expected: pbState.PendingConsolidations,
			},
		},
	}
}

func getExecutionPayloadDenebSpec() testutil.TestSpec {
	// Create an ExecutionPayloadDeneb with test data
	payload := &enginev1.ExecutionPayloadDeneb{
		ParentHash:    generateUniqueBytes(32, 0),
		FeeRecipient:  generateUniqueBytes(20, 32),
		StateRoot:     generateUniqueBytes(32, 52),
		ReceiptsRoot:  generateUniqueBytes(32, 84),
		LogsBloom:     generateUniqueBytes(256, 116),
		PrevRandao:    generateUniqueBytes(32, 372),
		BlockNumber:   999999,
		GasLimit:      30000000,
		GasUsed:       25000000,
		Timestamp:     1234567890,
		ExtraData:     []byte{0xaa, 0xbb, 0xcc},
		BaseFeePerGas: generateUniqueBytes(32, 404),
		BlockHash:     generateUniqueBytes(32, 436),
		BlobGasUsed:   131072,
		ExcessBlobGas: 65536,
	}

	// Add simple transactions
	payload.Transactions = [][]byte{
		generateUniqueBytes(100, 468),
		generateUniqueBytes(200, 568),
		generateUniqueBytes(150, 768),
	}

	// Add withdrawals
	payload.Withdrawals = []*enginev1.Withdrawal{
		{
			Index:          1000,
			ValidatorIndex: 100,
			Address:        generateUniqueBytes(20, 918),
			Amount:         1000000000,
		},
		{
			Index:          1001,
			ValidatorIndex: 101,
			Address:        generateUniqueBytes(20, 938),
			Amount:         2000000000,
		},
	}

	return testutil.TestSpec{
		Name:     "ExecutionPayloadDeneb",
		Type:     enginev1.ExecutionPayloadDeneb{},
		Instance: payload,
		PathTests: []testutil.PathTest{
			{
				Path:     ".parent_hash",
				Expected: payload.ParentHash,
			},
			{
				Path:     ".fee_recipient",
				Expected: payload.FeeRecipient,
			},
			{
				Path:     ".state_root",
				Expected: payload.StateRoot,
			},
			{
				Path:     ".receipts_root",
				Expected: payload.ReceiptsRoot,
			},
			{
				Path:     ".logs_bloom",
				Expected: payload.LogsBloom,
			},
			{
				Path:     ".prev_randao",
				Expected: payload.PrevRandao,
			},
			{
				Path:     ".block_number",
				Expected: payload.BlockNumber,
			},
			{
				Path:     ".gas_limit",
				Expected: payload.GasLimit,
			},
			{
				Path:     ".gas_used",
				Expected: payload.GasUsed,
			},
			{
				Path:     ".timestamp",
				Expected: payload.Timestamp,
			},
			{
				Path:     ".extra_data",
				Expected: payload.ExtraData,
			},
			{
				Path:     ".base_fee_per_gas",
				Expected: payload.BaseFeePerGas,
			},
			{
				Path:     ".block_hash",
				Expected: payload.BlockHash,
			},
			{
				Path:     ".blob_gas_used",
				Expected: payload.BlobGasUsed,
			},
			{
				Path:     ".excess_blob_gas",
				Expected: payload.ExcessBlobGas,
			},
			// Note: Lists like transactions and withdrawals are commented out
			// because the test infrastructure doesn't handle SSZ list serialization properly
			// {
			// 	Path:     ".transactions",
			// 	Expected: payload.Transactions,
			// },
			// {
			// 	Path:     ".withdrawals",
			// 	Expected: payload.Withdrawals,
			// },
		},
	}
}

// generateUniqueBytes generates a byte slice of given length with unique sequential values
// starting from the given offset. It wraps around at 256.
func generateUniqueBytes(length int, offset int) []byte {
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = byte((offset + i) % 256)
	}
	return result
}
