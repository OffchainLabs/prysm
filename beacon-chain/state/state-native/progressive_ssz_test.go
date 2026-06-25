package state_native

import (
	"context"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProgressiveSSZEnabled(t *testing.T) {
	reset := features.InitWithReset(&features.Flags{})
	defer reset()
	require.Equal(t, false, progressiveSSZEnabled(version.Gloas))

	reset = features.InitWithReset(&features.Flags{EnableProgressiveSSZ: true})
	defer reset()
	require.Equal(t, true, progressiveSSZEnabled(version.Gloas))
	require.Equal(t, false, progressiveSSZEnabled(version.Fulu))
}

func TestRootSelector_ProgressiveSSZGate(t *testing.T) {
	st := newGloasStateForProgressiveSSZTests(t)

	reset := features.InitWithReset(&features.Flags{})
	defer reset()

	legacyValidatorsRoot, err := st.rootSelector(context.Background(), types.Validators)
	require.NoError(t, err)
	expectedLegacyValidatorsRoot, err := stateutil.ValidatorRegistryRoot(st.validatorsCompactVal())
	require.NoError(t, err)
	require.Equal(t, expectedLegacyValidatorsRoot, legacyValidatorsRoot)

	legacyBalancesRoot, err := st.rootSelector(context.Background(), types.Balances)
	require.NoError(t, err)
	expectedLegacyBalancesRoot, err := stateutil.Uint64ListRootWithRegistryLimit(st.balancesVal())
	require.NoError(t, err)
	require.Equal(t, expectedLegacyBalancesRoot, legacyBalancesRoot)

	legacyExpectedWithdrawalsRoot, err := st.rootSelector(context.Background(), types.PayloadExpectedWithdrawals)
	require.NoError(t, err)
	expectedLegacyExpectedWithdrawalsRoot, err := ssz.WithdrawalSliceRoot(st.payloadExpectedWithdrawals, fieldparams.MaxWithdrawalsPerPayload)
	require.NoError(t, err)
	require.Equal(t, expectedLegacyExpectedWithdrawalsRoot, legacyExpectedWithdrawalsRoot)

	legacyBuilderPendingWithdrawalsRoot, err := st.rootSelector(context.Background(), types.BuilderPendingWithdrawals)
	require.NoError(t, err)
	expectedLegacyBuilderPendingWithdrawalsRoot, err := stateutil.BuilderPendingWithdrawalsRoot(st.builderPendingWithdrawals)
	require.NoError(t, err)
	require.Equal(t, expectedLegacyBuilderPendingWithdrawalsRoot, legacyBuilderPendingWithdrawalsRoot)

	legacyBuildersRoot, err := st.rootSelector(context.Background(), types.Builders)
	require.NoError(t, err)
	expectedLegacyBuildersRoot, err := stateutil.BuildersRoot(st.builders)
	require.NoError(t, err)
	require.Equal(t, expectedLegacyBuildersRoot, legacyBuildersRoot)

	reset = features.InitWithReset(&features.Flags{EnableProgressiveSSZ: true})
	defer reset()

	progressiveValidatorsRoot, err := st.rootSelector(context.Background(), types.Validators)
	require.NoError(t, err)
	expectedProgressiveValidatorsRoot, err := stateutil.ValidatorRegistryRootProgressive(st.validatorsCompactVal())
	require.NoError(t, err)
	require.Equal(t, expectedProgressiveValidatorsRoot, progressiveValidatorsRoot)
	require.DeepNotSSZEqual(t, legacyValidatorsRoot, progressiveValidatorsRoot)

	progressiveBalancesRoot, err := st.rootSelector(context.Background(), types.Balances)
	require.NoError(t, err)
	expectedProgressiveBalancesRoot, err := stateutil.Uint64ListRootProgressive(st.balancesVal())
	require.NoError(t, err)
	require.Equal(t, expectedProgressiveBalancesRoot, progressiveBalancesRoot)
	require.DeepNotSSZEqual(t, legacyBalancesRoot, progressiveBalancesRoot)

	progressiveExpectedWithdrawalsRoot, err := st.rootSelector(context.Background(), types.PayloadExpectedWithdrawals)
	require.NoError(t, err)
	expectedProgressiveExpectedWithdrawalsRoot, err := ssz.WithdrawalSliceRootProgressive(st.payloadExpectedWithdrawals, fieldparams.MaxWithdrawalsPerPayload)
	require.NoError(t, err)
	require.Equal(t, expectedProgressiveExpectedWithdrawalsRoot, progressiveExpectedWithdrawalsRoot)
	require.DeepNotSSZEqual(t, legacyExpectedWithdrawalsRoot, progressiveExpectedWithdrawalsRoot)

	progressiveBuilderPendingWithdrawalsRoot, err := st.rootSelector(context.Background(), types.BuilderPendingWithdrawals)
	require.NoError(t, err)
	expectedProgressiveBuilderPendingWithdrawalsRoot, err := stateutil.BuilderPendingWithdrawalsRootProgressive(st.builderPendingWithdrawals)
	require.NoError(t, err)
	require.Equal(t, expectedProgressiveBuilderPendingWithdrawalsRoot, progressiveBuilderPendingWithdrawalsRoot)
	require.DeepNotSSZEqual(t, legacyBuilderPendingWithdrawalsRoot, progressiveBuilderPendingWithdrawalsRoot)

	progressiveBuildersRoot, err := st.rootSelector(context.Background(), types.Builders)
	require.NoError(t, err)
	expectedProgressiveBuildersRoot, err := stateutil.BuildersRootProgressive(st.builders)
	require.NoError(t, err)
	require.Equal(t, expectedProgressiveBuildersRoot, progressiveBuildersRoot)
	require.DeepNotSSZEqual(t, legacyBuildersRoot, progressiveBuildersRoot)
}

func TestComputeFieldRootsWithHasher_ProgressiveSSZGate(t *testing.T) {
	st := newGloasStateForProgressiveSSZTests(t)

	reset := features.InitWithReset(&features.Flags{})
	defer reset()

	legacyRoots, err := ComputeFieldRootsWithHasher(context.Background(), st)
	require.NoError(t, err)
	expectedLegacyPendingDepositsRoot, err := stateutil.PendingDepositsRoot(st.pendingDeposits)
	require.NoError(t, err)
	require.DeepEqual(t, expectedLegacyPendingDepositsRoot[:], legacyRoots[types.PendingDeposits.RealPosition()])
	expectedLegacyWithdrawalsRoot, err := ssz.WithdrawalSliceRoot(st.payloadExpectedWithdrawals, fieldparams.MaxWithdrawalsPerPayload)
	require.NoError(t, err)
	require.DeepEqual(t, expectedLegacyWithdrawalsRoot[:], legacyRoots[types.PayloadExpectedWithdrawals.RealPosition()])
	expectedLegacyBuilderPendingWithdrawalsRoot, err := stateutil.BuilderPendingWithdrawalsRoot(st.builderPendingWithdrawals)
	require.NoError(t, err)
	require.DeepEqual(t, expectedLegacyBuilderPendingWithdrawalsRoot[:], legacyRoots[types.BuilderPendingWithdrawals.RealPosition()])
	expectedLegacyBuildersRoot, err := stateutil.BuildersRoot(st.builders)
	require.NoError(t, err)
	require.DeepEqual(t, expectedLegacyBuildersRoot[:], legacyRoots[types.Builders.RealPosition()])

	reset = features.InitWithReset(&features.Flags{EnableProgressiveSSZ: true})
	defer reset()

	progressiveRoots, err := ComputeFieldRootsWithHasher(context.Background(), st)
	require.NoError(t, err)
	expectedProgressivePendingDepositsRoot, err := stateutil.PendingDepositsRootProgressive(st.pendingDeposits)
	require.NoError(t, err)
	require.DeepEqual(t, expectedProgressivePendingDepositsRoot[:], progressiveRoots[types.PendingDeposits.RealPosition()])
	require.DeepNotSSZEqual(t, legacyRoots[types.PendingDeposits.RealPosition()], progressiveRoots[types.PendingDeposits.RealPosition()])
	expectedProgressiveWithdrawalsRoot, err := ssz.WithdrawalSliceRootProgressive(st.payloadExpectedWithdrawals, fieldparams.MaxWithdrawalsPerPayload)
	require.NoError(t, err)
	require.DeepEqual(t, expectedProgressiveWithdrawalsRoot[:], progressiveRoots[types.PayloadExpectedWithdrawals.RealPosition()])
	require.DeepNotSSZEqual(t, legacyRoots[types.PayloadExpectedWithdrawals.RealPosition()], progressiveRoots[types.PayloadExpectedWithdrawals.RealPosition()])
	expectedProgressiveBuilderPendingWithdrawalsRoot, err := stateutil.BuilderPendingWithdrawalsRootProgressive(st.builderPendingWithdrawals)
	require.NoError(t, err)
	require.DeepEqual(t, expectedProgressiveBuilderPendingWithdrawalsRoot[:], progressiveRoots[types.BuilderPendingWithdrawals.RealPosition()])
	require.DeepNotSSZEqual(t, legacyRoots[types.BuilderPendingWithdrawals.RealPosition()], progressiveRoots[types.BuilderPendingWithdrawals.RealPosition()])
	expectedProgressiveBuildersRoot, err := stateutil.BuildersRootProgressive(st.builders)
	require.NoError(t, err)
	require.DeepEqual(t, expectedProgressiveBuildersRoot[:], progressiveRoots[types.Builders.RealPosition()])
	require.DeepNotSSZEqual(t, legacyRoots[types.Builders.RealPosition()], progressiveRoots[types.Builders.RealPosition()])
}

func TestHashTreeRoot_ProgressiveSSZGate(t *testing.T) {
	st := newGloasStateForProgressiveSSZTests(t)

	reset := features.InitWithReset(&features.Flags{})
	defer reset()

	legacyRoot, err := st.HashTreeRoot(context.Background())
	require.NoError(t, err)

	legacyFieldRoots, err := ComputeFieldRootsWithHasher(context.Background(), st)
	require.NoError(t, err)
	legacyLayers := stateutil.Merkleize(legacyFieldRoots)
	expectedLegacyRoot := bytesutil.ToBytes32(legacyLayers[len(legacyLayers)-1][0])
	require.Equal(t, expectedLegacyRoot, legacyRoot)

	reset = features.InitWithReset(&features.Flags{EnableProgressiveSSZ: true})
	defer reset()

	progressiveRoot, err := st.HashTreeRoot(context.Background())
	require.NoError(t, err)

	progressiveFieldRootsBytes, err := ComputeFieldRootsWithHasher(context.Background(), st)
	require.NoError(t, err)
	progressiveFieldRoots := make([][32]byte, len(progressiveFieldRootsBytes))
	for i := range progressiveFieldRootsBytes {
		progressiveFieldRoots[i] = bytesutil.ToBytes32(progressiveFieldRootsBytes[i])
	}

	activeFields := make([]bool, len(progressiveFieldRoots))
	for i := range activeFields {
		activeFields[i] = true
	}

	expectedProgressiveRoot, err := ssz.ContainerRootProgressive(progressiveFieldRoots, activeFields)
	require.NoError(t, err)
	require.Equal(t, expectedProgressiveRoot, progressiveRoot)
	require.DeepNotSSZEqual(t, legacyRoot, progressiveRoot)

	progressiveRootAgain, err := st.HashTreeRoot(context.Background())
	require.NoError(t, err)
	require.Equal(t, progressiveRoot, progressiveRootAgain)
}

func newGloasStateForProgressiveSSZTests(t *testing.T) *BeaconState {
	t.Helper()

	pubkeys := make([][]byte, 512)
	for i := range pubkeys {
		pubkeys[i] = make([]byte, fieldparams.BLSPubkeyLength)
	}

	builderPendingPayments := make([]*ethpb.BuilderPendingPayment, 64)
	for i := range builderPendingPayments {
		builderPendingPayments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
			},
		}
	}

	builderPendingWithdrawals := []*ethpb.BuilderPendingWithdrawal{
		{
			FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
			Amount:       9,
			BuilderIndex: 1,
		},
		{
			FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
			Amount:       10,
			BuilderIndex: 2,
		},
	}
	builderPendingWithdrawals[0].FeeRecipient[0] = 1
	builderPendingWithdrawals[1].FeeRecipient[0] = 2

	ptcWindow := make([]*ethpb.PTCs, 3*params.BeaconConfig().SlotsPerEpoch)
	for i := range ptcWindow {
		ptcWindow[i] = &ethpb.PTCs{
			ValidatorIndices: make([]primitives.ValidatorIndex, fieldparams.PTCSize),
		}
	}

	pubkey1 := make([]byte, fieldparams.BLSPubkeyLength)
	pubkey1[0] = 1
	pubkey2 := make([]byte, fieldparams.BLSPubkeyLength)
	pubkey2[0] = 2
	withdrawalCredentials := make([]byte, fieldparams.RootLength)
	signature := make([]byte, fieldparams.BLSSignatureLength)

	st, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		BlockRoots:  filledByteSlice2D(uint64(params.BeaconConfig().SlotsPerHistoricalRoot), fieldparams.RootLength),
		StateRoots:  filledByteSlice2D(uint64(params.BeaconConfig().SlotsPerHistoricalRoot), fieldparams.RootLength),
		Slashings:   make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector),
		RandaoMixes: filledByteSlice2D(uint64(params.BeaconConfig().EpochsPerHistoricalVector), fieldparams.RootLength),
		Validators: []*ethpb.Validator{
			{PublicKey: pubkey1, WithdrawalCredentials: withdrawalCredentials, EffectiveBalance: 32},
			{PublicKey: pubkey2, WithdrawalCredentials: withdrawalCredentials, EffectiveBalance: 64, Slashed: true},
		},
		Balances: []uint64{33, 65},
		CurrentJustifiedCheckpoint: &ethpb.Checkpoint{
			Root: make([]byte, fieldparams.RootLength),
		},
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		Fork: &ethpb.Fork{
			PreviousVersion: make([]byte, 4),
			CurrentVersion:  make([]byte, 4),
		},
		Eth1DataVotes:       make([]*ethpb.Eth1Data, 0),
		HistoricalRoots:     make([][]byte, 0),
		JustificationBits:   bitfield.Bitvector4{0x0},
		FinalizedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		LatestBlockHeader:   &ethpb.BeaconBlockHeader{},
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{
			Root: make([]byte, fieldparams.RootLength),
		},
		PreviousEpochParticipation: []byte{0x01, 0x02},
		CurrentEpochParticipation:  []byte{0x03, 0x04, 0x05},
		InactivityScores:           []uint64{7, 8},
		CurrentSyncCommittee: &ethpb.SyncCommittee{
			Pubkeys:         pubkeys,
			AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength),
		},
		NextSyncCommittee: &ethpb.SyncCommittee{
			Pubkeys:         pubkeys,
			AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength),
		},
		PendingDeposits: []*ethpb.PendingDeposit{{
			PublicKey:             pubkey1,
			WithdrawalCredentials: withdrawalCredentials,
			Amount:                1,
			Signature:             signature,
		}},
		PendingPartialWithdrawals: []*ethpb.PendingPartialWithdrawal{
			{Index: 1, Amount: 2},
			{Index: 0, Amount: 3},
		},
		PendingConsolidations: []*ethpb.PendingConsolidation{
			{SourceIndex: 1, TargetIndex: 0},
			{SourceIndex: 0, TargetIndex: 1},
		},
		ProposerLookahead: make([]primitives.ValidatorIndex, 64),
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
			ParentBlockHash:       make([]byte, fieldparams.RootLength),
			ParentBlockRoot:       make([]byte, fieldparams.RootLength),
			BlockHash:             make([]byte, fieldparams.RootLength),
			PrevRandao:            make([]byte, fieldparams.RootLength),
			FeeRecipient:          make([]byte, fieldparams.FeeRecipientLength),
			BlobKzgCommitments:    [][]byte{make([]byte, fieldparams.BLSPubkeyLength)},
			ExecutionRequestsRoot: make([]byte, fieldparams.RootLength),
		},
		Builders: []*ethpb.Builder{{
			Pubkey:            pubkey1,
			Version:           []byte{1},
			ExecutionAddress:  make([]byte, fieldparams.FeeRecipientLength),
			Balance:           11,
			DepositEpoch:      12,
			WithdrawableEpoch: 13,
		}},
		ExecutionPayloadAvailability: make([]byte, 1024),
		BuilderPendingPayments:       builderPendingPayments,
		BuilderPendingWithdrawals:    builderPendingWithdrawals,
		LatestBlockHash:              make([]byte, fieldparams.RootLength),
		PayloadExpectedWithdrawals:   make([]*enginev1.Withdrawal, 0),
		PtcWindow:                    ptcWindow,
	})
	require.NoError(t, err)

	bs, ok := st.(*BeaconState)
	require.Equal(t, true, ok)
	return bs
}

func filledByteSlice2D(length uint64, innerLen int) [][]byte {
	b := make([][]byte, length)
	for i := range b {
		b[i] = make([]byte, innerLen)
	}
	return b
}
