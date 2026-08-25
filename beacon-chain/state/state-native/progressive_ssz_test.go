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
	require.Equal(t, false, features.ProgressiveSSZEnabled(version.Gloas))

	reset = features.InitWithReset(&features.Flags{EnableProgressiveSSZ: true})
	defer reset()
	require.Equal(t, true, features.ProgressiveSSZEnabled(version.Gloas))
	require.Equal(t, false, features.ProgressiveSSZEnabled(version.Fulu))
}

func TestComputeFieldRootsWithHasher_ProgressiveSSZFields(t *testing.T) {
	ctx := context.Background()
	st := newGloasStateForProgressiveSSZTests(t)

	tests := []struct {
		name        string
		progressive bool
	}{
		{name: "legacy"},
		{name: "progressive", progressive: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{EnableProgressiveSSZ: tt.progressive})
			defer reset()

			roots, err := ComputeFieldRootsWithHasher(context.Background(), st)
			require.NoError(t, err)

			var pendingDepositsRoot, pendingPartialWithdrawalsRoot, pendingConsolidationsRoot, expectedWithdrawalsRoot, buildersRoot [32]byte
			if tt.progressive {
				pendingDepositsRoot, err = ssz.SliceRootProgressive(st.pendingDeposits)
				require.NoError(t, err)
				pendingPartialWithdrawalsRoot, err = ssz.SliceRootProgressive(st.pendingPartialWithdrawals)
				require.NoError(t, err)
				pendingConsolidationsRoot, err = ssz.SliceRootProgressive(st.pendingConsolidations)
				require.NoError(t, err)
				expectedWithdrawalsRoot, err = ssz.SliceRootProgressive(st.payloadExpectedWithdrawals)
				require.NoError(t, err)
				buildersRoot, err = stateutil.BuildersRoot(version.Gloas, st.builders)
				require.NoError(t, err)
			} else {
				pendingDepositsRoot, err = ssz.SliceRoot(st.pendingDeposits, fieldparams.PendingDepositsLimit)
				require.NoError(t, err)
				pendingPartialWithdrawalsRoot, err = ssz.SliceRoot(st.pendingPartialWithdrawals, fieldparams.PendingPartialWithdrawalsLimit)
				require.NoError(t, err)
				pendingConsolidationsRoot, err = ssz.SliceRoot(st.pendingConsolidations, fieldparams.PendingConsolidationsLimit)
				require.NoError(t, err)
				expectedWithdrawalsRoot, err = ssz.SliceRoot(st.payloadExpectedWithdrawals, fieldparams.MaxWithdrawalsPerPayload)
				require.NoError(t, err)
				buildersRoot, err = ssz.SliceRoot(st.builders, fieldparams.BuilderRegistryLimit)
				require.NoError(t, err)
			}

			require.DeepEqual(t, pendingDepositsRoot[:], roots[types.PendingDeposits.RealPosition()])
			require.DeepEqual(t, pendingPartialWithdrawalsRoot[:], roots[types.PendingPartialWithdrawals.RealPosition()])
			require.DeepEqual(t, pendingConsolidationsRoot[:], roots[types.PendingConsolidations.RealPosition()])
			require.DeepEqual(t, expectedWithdrawalsRoot[:], roots[types.PayloadExpectedWithdrawals.RealPosition()])
			require.DeepEqual(t, buildersRoot[:], roots[types.Builders.RealPosition()])

			rootSelectorPendingDepositsRoot, err := st.rootSelector(ctx, types.PendingDeposits)
			require.NoError(t, err)
			require.DeepEqual(t, rootSelectorPendingDepositsRoot[:], roots[types.PendingDeposits.RealPosition()])

			rootSelectorPendingPartialWithdrawalsRoot, err := st.rootSelector(ctx, types.PendingPartialWithdrawals)
			require.NoError(t, err)
			require.DeepEqual(t, rootSelectorPendingPartialWithdrawalsRoot[:], roots[types.PendingPartialWithdrawals.RealPosition()])

			rootSelectorPendingConsolidationsRoot, err := st.rootSelector(ctx, types.PendingConsolidations)
			require.NoError(t, err)
			require.DeepEqual(t, rootSelectorPendingConsolidationsRoot[:], roots[types.PendingConsolidations.RealPosition()])

			rootSelectorExpectedWithdrawalsRoot, err := st.rootSelector(ctx, types.PayloadExpectedWithdrawals)
			require.NoError(t, err)
			require.DeepEqual(t, rootSelectorExpectedWithdrawalsRoot[:], roots[types.PayloadExpectedWithdrawals.RealPosition()])

			rootSelectorBuildersRoot, err := st.rootSelector(ctx, types.Builders)
			require.NoError(t, err)
			require.DeepEqual(t, rootSelectorBuildersRoot[:], roots[types.Builders.RealPosition()])
		})
	}
}

func TestHashTreeRoot(t *testing.T) {
	t.Run("UnsupportedVersion", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.initializeProgressiveMerkleTree(t.Context())
		require.ErrorContains(t, "unsupported version: fulu", err)

		_, err = st.progressiveHashTreeRoot(t.Context())
		require.ErrorContains(t, "progressive SSZ is not supported for version: fulu", err)
	})

	t.Run("ProgressiveSSZGate", func(t *testing.T) {
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
	})

	t.Run("ProgressiveSSZIncremental", func(t *testing.T) {
		reset := features.InitWithReset(&features.Flags{EnableProgressiveSSZ: true})
		defer reset()

		st := newGloasStateForProgressiveSSZTests(t)
		initialRoot, err := st.HashTreeRoot(context.Background())
		require.NoError(t, err)
		require.Equal(t, 0, len(st.dirtyFields))
		if st.progressiveMerkleTree == nil {
			t.Fatal("progressive Merkle tree was not initialized")
		}
		cachedTree := st.progressiveMerkleTree

		// Initial progressive merkleization warms the validators field trie.
		// Updating only the slot must not replace that unrelated trie.
		require.Equal(t, false, st.rebuildTrie[types.Validators])
		validatorsTrie := st.stateFieldLeaves[types.Validators]
		require.NoError(t, st.SetSlot(st.slot+1))
		require.Equal(t, true, st.dirtyFields[types.Slot])

		updatedRoot, err := st.HashTreeRoot(context.Background())
		require.NoError(t, err)
		require.DeepNotSSZEqual(t, initialRoot, updatedRoot)
		require.Equal(t, progressiveRootFromScratch(t, st), updatedRoot)
		require.Equal(t, 0, len(st.dirtyFields))
		require.Equal(t, validatorsTrie, st.stateFieldLeaves[types.Validators])
		if cachedTree != st.progressiveMerkleTree {
			t.Fatal("progressive Merkle tree was rebuilt instead of updated in place")
		}

		unchangedRoot, err := st.HashTreeRoot(context.Background())
		require.NoError(t, err)
		require.Equal(t, updatedRoot, unchangedRoot)
		if cachedTree != st.progressiveMerkleTree {
			t.Fatal("unchanged progressive Merkle tree was rebuilt")
		}
	})

	t.Run("ProgressiveSSZCopy", func(t *testing.T) {
		reset := features.InitWithReset(&features.Flags{EnableProgressiveSSZ: true})
		defer reset()

		st := newGloasStateForProgressiveSSZTests(t)
		originalRoot, err := st.HashTreeRoot(context.Background())
		require.NoError(t, err)

		copied, ok := st.Copy().(*BeaconState)
		require.Equal(t, true, ok)
		if copied.progressiveMerkleTree == nil {
			t.Fatal("copied state did not retain the progressive Merkle cache")
		}
		if copied.progressiveMerkleTree == st.progressiveMerkleTree {
			t.Fatal("copied state shares its mutable progressive Merkle cache")
		}

		copiedRoot, err := copied.HashTreeRoot(context.Background())
		require.NoError(t, err)
		require.Equal(t, originalRoot, copiedRoot)

		require.NoError(t, copied.SetSlot(copied.slot+1))
		mutatedCopyRoot, err := copied.HashTreeRoot(context.Background())
		require.NoError(t, err)
		require.DeepNotSSZEqual(t, originalRoot, mutatedCopyRoot)
		require.Equal(t, progressiveRootFromScratch(t, copied), mutatedCopyRoot)

		originalRootAgain, err := st.HashTreeRoot(context.Background())
		require.NoError(t, err)
		require.Equal(t, originalRoot, originalRootAgain)
	})
}

func progressiveRootFromScratch(t *testing.T, st *BeaconState) [32]byte {
	t.Helper()

	fieldRootsBytes, err := ComputeFieldRootsWithHasher(context.Background(), st)
	require.NoError(t, err)
	fieldRoots := make([][32]byte, len(fieldRootsBytes))
	for i := range fieldRootsBytes {
		fieldRoots[i] = bytesutil.ToBytes32(fieldRootsBytes[i])
	}

	activeFields := make([]bool, len(fieldRoots))
	for i := range activeFields {
		activeFields[i] = true
	}
	root, err := ssz.ContainerRootProgressive(fieldRoots, activeFields)
	require.NoError(t, err)
	return root
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
		BuilderPendingWithdrawals:    make([]*ethpb.BuilderPendingWithdrawal, 0),
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
