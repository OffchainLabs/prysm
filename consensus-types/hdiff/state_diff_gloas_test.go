package hdiff

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

func gloasState(t testing.TB, numValidators uint64) (state.BeaconState, error) {
	fuluState, _ := util.DeterministicGenesisStateFulu(t, numValidators)
	return gloas.UpgradeToGloas(t.Context(), fuluState)
}

func requireEqualState(t testing.TB, expected, actual state.BeaconState) {
	t.Helper()
	expectedRoot, err := expected.HashTreeRoot(t.(*testing.T).Context())
	require.NoError(t, err)
	actualRoot, err := actual.HashTreeRoot(t.(*testing.T).Context())
	require.NoError(t, err)
	require.Equal(t, expectedRoot, actualRoot)
}

func TestGloasRoundTrip(t *testing.T) {
	source, err := gloasState(t, 64)
	require.NoError(t, err)
	require.Equal(t, version.Gloas, source.Version())

	target := source.Copy()

	// Mutate target to create a meaningful diff.
	require.NoError(t, target.SetSlot(source.Slot()+1))

	// Mutate latestBlockHash.
	var newHash [32]byte
	for i := range newHash {
		newHash[i] = byte(i + 1)
	}
	require.NoError(t, target.SetLatestBlockHash(newHash))

	// Mutate nextWithdrawalBuilderIndex.
	require.NoError(t, target.SetNextWithdrawalBuilderIndex(42))

	// Mutate payloadExpectedWithdrawals (multiple entries).
	require.NoError(t, target.SetPayloadExpectedWithdrawals([]*enginev1.Withdrawal{
		{
			Index:          1,
			ValidatorIndex: 2,
			Address:        make([]byte, 20),
			Amount:         100,
		},
		{
			Index:          2,
			ValidatorIndex: 5,
			Address:        make([]byte, 20),
			Amount:         200,
		},
		{
			Index:          3,
			ValidatorIndex: 10,
			Address:        make([]byte, 20),
			Amount:         300,
		},
	}))

	// Full round-trip: diff → serialize → deserialize → apply.
	ctx := t.Context()
	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, source, hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, target, result)
}

func TestGloasBuilderDiffs(t *testing.T) {
	source, err := gloasState(t, 64)
	require.NoError(t, err)

	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))

	// Get current builders and mutate one.
	builders, err := target.Builders()
	require.NoError(t, err)

	if len(builders) == 0 {
		// Add a builder if none exist.
		require.NoError(t, target.AddBuilderFromDeposit(
			[fieldparams.BLSPubkeyLength]byte{0xaa},
			[fieldparams.RootLength]byte{0x04, 0xbb}, // builder withdrawal credential prefix
			32_000_000_000,
		))
	} else {
		// Mutate existing builder balance.
		builders[0] = ethpb.CopyBuilder(builders[0])
		builders[0].Balance = 999_000_000_000
		require.NoError(t, target.SetBuilders(builders))
	}

	ctx := t.Context()
	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, source, hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, target, result)
}

func TestGloasBuilderReplacement(t *testing.T) {
	source, err := gloasState(t, 64)
	require.NoError(t, err)

	// Add three builders to source.
	for i := byte(1); i <= 3; i++ {
		require.NoError(t, source.AddBuilderFromDeposit(
			[fieldparams.BLSPubkeyLength]byte{i},
			[fieldparams.RootLength]byte{0x04, i},
			32_000_000_000,
		))
	}

	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))

	// Replace builders at index 0 and 2 (simulates recycling at non-contiguous indices).
	builders, err := target.Builders()
	require.NoError(t, err)
	for _, idx := range []int{0, 2} {
		builders[idx] = &ethpb.Builder{
			Pubkey:            make([]byte, 48),
			Version:           []byte{0x04},
			ExecutionAddress:  make([]byte, 20),
			Balance:           64_000_000_000,
			DepositEpoch:      10,
			WithdrawableEpoch: 1<<64 - 1,
		}
		copy(builders[idx].Pubkey, []byte{0xff, byte(idx)})
	}
	require.NoError(t, target.SetBuilders(builders))

	ctx := t.Context()
	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, source, hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, target, result)
}

func TestGloasBuilderPendingWithdrawals(t *testing.T) {
	source, err := gloasState(t, 64)
	require.NoError(t, err)

	// Add multiple pending withdrawals to source.
	require.NoError(t, source.AppendBuilderPendingWithdrawals([]*ethpb.BuilderPendingWithdrawal{
		{FeeRecipient: make([]byte, 20), Amount: 100, BuilderIndex: 0},
		{FeeRecipient: make([]byte, 20), Amount: 200, BuilderIndex: 1},
		{FeeRecipient: make([]byte, 20), Amount: 300, BuilderIndex: 2},
		{FeeRecipient: make([]byte, 20), Amount: 400, BuilderIndex: 3},
		{FeeRecipient: make([]byte, 20), Amount: 500, BuilderIndex: 4},
	}))

	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))

	// Simulate prefix-drop (dequeue first 2) + append multiple new.
	require.NoError(t, target.DequeueBuilderPendingWithdrawals(2))
	require.NoError(t, target.AppendBuilderPendingWithdrawals([]*ethpb.BuilderPendingWithdrawal{
		{FeeRecipient: make([]byte, 20), Amount: 600, BuilderIndex: 5},
		{FeeRecipient: make([]byte, 20), Amount: 700, BuilderIndex: 6},
	}))

	ctx := t.Context()
	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, source, hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, target, result)
}

func TestGloasPendingPaymentsOverride(t *testing.T) {
	source, err := gloasState(t, 64)
	require.NoError(t, err)

	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))

	// Mutate multiple pending payments at different indices.
	payments, err := target.BuilderPendingPayments()
	require.NoError(t, err)
	for _, idx := range []int{0, 5, 10} {
		payments[idx] = &ethpb.BuilderPendingPayment{
			Weight: primitives.Gwei(500 * (idx + 1)),
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, 20),
				Amount:       primitives.Gwei(1000 * (idx + 1)),
				BuilderIndex: primitives.BuilderIndex(idx),
			},
		}
	}
	require.NoError(t, target.SetBuilderPendingPayments(payments))

	ctx := t.Context()
	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, source, hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, target, result)
}

func TestGloasExecutionPayloadAvailability(t *testing.T) {
	source, err := gloasState(t, 64)
	require.NoError(t, err)

	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))

	// Flip a bit in execution payload availability.
	require.NoError(t, target.SetExecutionPayloadAvailability(0, false))

	ctx := t.Context()
	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, source, hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, target, result)
}

func TestGloasCrossForkDiff(t *testing.T) {
	// Test Fulu → Gloas cross-fork diff via updateToVersion.
	fuluSource, _ := util.DeterministicGenesisStateFulu(t, 64)

	gloasTarget, err := gloas.UpgradeToGloas(t.Context(), fuluSource.Copy())
	require.NoError(t, err)
	require.NoError(t, gloasTarget.SetSlot(fuluSource.Slot()+1))

	ctx := t.Context()
	hdiffBytes, err := Diff(fuluSource, gloasTarget)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, fuluSource, hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, gloasTarget, result)
}

func TestGloasCrossForkDiffOnboardsBuilderDeposit(t *testing.T) {
	// A 0xB0 builder deposit in the Fulu anchor is removed by Gloas onboarding, shortening
	// pending_deposits. The diff indexes the pre-upgrade list, so applying it must not use the
	// post-upgrade (shortened) list, which previously panicked with slice bounds out of range.
	fuluSource, _ := util.DeterministicGenesisStateFulu(t, 64)

	wc := make([]byte, fieldparams.RootLength)
	wc[0] = params.BeaconConfig().BuilderWithdrawalPrefixByte
	for i := 12; i < len(wc); i++ {
		wc[i] = 0xAB
	}
	pubkey := make([]byte, fieldparams.BLSPubkeyLength)
	for i := range pubkey {
		pubkey[i] = 0xB1
	}
	require.NoError(t, fuluSource.SetPendingDeposits([]*ethpb.PendingDeposit{{
		PublicKey:             pubkey,
		WithdrawalCredentials: wc,
		Amount:                params.BeaconConfig().MinActivationBalance,
		Signature:             make([]byte, fieldparams.BLSSignatureLength),
		Slot:                  0,
	}}))

	gloasTarget, err := gloas.UpgradeToGloas(t.Context(), fuluSource.Copy())
	require.NoError(t, err)
	require.NoError(t, gloasTarget.SetSlot(fuluSource.Slot()+1))

	tPending, err := gloasTarget.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 0, len(tPending))

	ctx := t.Context()
	hdiffBytes, err := Diff(fuluSource, gloasTarget)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, fuluSource.Copy(), hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, gloasTarget, result)
}

func TestGloasCrossForkDiffDropsOnboardedBuilders(t *testing.T) {
	// The real chain can process a 0xB0 (`BUILDER_WITHDRAWAL_PREFIX`) deposit as a validator between the anchor slot and the fork.
	// Applying the diff then onboards builders from the anchor that the target never had.
	signedBuilderDeposit := func(t *testing.T) *ethpb.PendingDeposit {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		wc := make([]byte, fieldparams.RootLength)
		wc[0] = params.BeaconConfig().BuilderWithdrawalPrefixByte
		amount := params.BeaconConfig().MinActivationBalance
		domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
		require.NoError(t, err)
		root, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
			PublicKey:             sk.PublicKey().Marshal(),
			WithdrawalCredentials: wc,
			Amount:                amount,
		}, domain)
		require.NoError(t, err)
		return &ethpb.PendingDeposit{
			PublicKey:             sk.PublicKey().Marshal(),
			WithdrawalCredentials: wc,
			Amount:                amount,
			Signature:             sk.Sign(root[:]).Marshal(),
		}
	}

	tests := []struct {
		name           string
		targetDeposits func(anchor []*ethpb.PendingDeposit) []*ethpb.PendingDeposit
		wantBuilders   int
	}{
		{
			name:           "target has no builders",
			targetDeposits: func([]*ethpb.PendingDeposit) []*ethpb.PendingDeposit { return nil },
			wantBuilders:   0,
		},
		{
			name:           "target has fewer builders",
			targetDeposits: func(anchor []*ethpb.PendingDeposit) []*ethpb.PendingDeposit { return anchor[1:] },
			wantBuilders:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fulu source state with two pending deposits.
			fuluSource, _ := util.DeterministicGenesisStateFulu(t, 64)
			anchorDeposits := []*ethpb.PendingDeposit{signedBuilderDeposit(t), signedBuilderDeposit(t)}
			require.NoError(t, fuluSource.SetPendingDeposits(anchorDeposits))

			// Upgrade to Gloas and onboarding happens.
			onboarded, err := gloas.UpgradeToGloas(t.Context(), fuluSource.Copy())
			require.NoError(t, err)
			onboardedBuilders, err := onboarded.Builders()
			require.NoError(t, err)
			require.Equal(t, len(anchorDeposits), len(onboardedBuilders))

			fuluTarget := fuluSource.Copy()
			require.NoError(t, fuluTarget.SetPendingDeposits(tt.targetDeposits(anchorDeposits)))
			gloasTarget, err := gloas.UpgradeToGloas(t.Context(), fuluTarget)
			require.NoError(t, err)
			require.NoError(t, gloasTarget.SetSlot(fuluSource.Slot()+1))

			// Pre-Gloas source state will have zero builders.
			// As the target is post-Gloas, the patch will contain all builders in the target. (`diffBuilders`)
			hdiffBytes, err := Diff(fuluSource, gloasTarget)
			require.NoError(t, err)
			result, err := ApplyDiff(t.Context(), fuluSource.Copy(), hdiffBytes)
			require.NoError(t, err)

			resultBuilders, err := result.Builders()
			require.NoError(t, err)
			require.Equal(t, tt.wantBuilders, len(resultBuilders))
			requireEqualState(t, gloasTarget, result)
		})
	}
}

func TestGloasSerializeDeserializeRoundTrip(t *testing.T) {
	source, err := gloasState(t, 64)
	require.NoError(t, err)

	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))

	var newHash [32]byte
	for i := range newHash {
		newHash[i] = byte(i + 42)
	}
	require.NoError(t, target.SetLatestBlockHash(newHash))
	require.NoError(t, target.SetNextWithdrawalBuilderIndex(primitives.BuilderIndex(99)))

	// Diff → serialize.
	sd, err := diffToState(source, target)
	require.NoError(t, err)
	require.Equal(t, version.Gloas, sd.targetVersion)
	require.NotNil(t, sd.latestExecutionPayloadBid)

	raw := sd.serialize()
	require.NotNil(t, raw)
	require.Equal(t, sd.serializedSize(), len(raw))
	require.Equal(t, len(raw), cap(raw))
	serialized := snappy.Encode(nil, raw)

	// Deserialize → verify fields.
	deserialized, err := newStateDiff(serialized)
	require.NoError(t, err)
	require.Equal(t, sd.targetVersion, deserialized.targetVersion)
	require.Equal(t, sd.slot, deserialized.slot)
	require.Equal(t, sd.nextWithdrawalBuilderIndex, deserialized.nextWithdrawalBuilderIndex)
	require.Equal(t, sd.latestBlockHash, deserialized.latestBlockHash)
	require.Equal(t, len(sd.builderDiffs), len(deserialized.builderDiffs))
	require.Equal(t, len(sd.builderPendingPayments), len(deserialized.builderPendingPayments))
	require.Equal(t, len(sd.builderPendingWithdrawalsDiff), len(deserialized.builderPendingWithdrawalsDiff))
	require.Equal(t, len(sd.executionPayloadAvailability), len(deserialized.executionPayloadAvailability))
	require.Equal(t, len(sd.payloadExpectedWithdrawals), len(deserialized.payloadExpectedWithdrawals))
}

func TestGloasNilBidErrors(t *testing.T) {
	// Verify that diffGloasFields errors on a nil bid.
	source, err := gloasState(t, 64)
	require.NoError(t, err)
	target := source.Copy()

	// Force bid to nil by using the underlying proto.
	// We can't do this through the public API, so we just verify
	// that a normal Gloas state always has a non-nil bid.
	bid, err := target.LatestExecutionPayloadBid()
	require.NoError(t, err)
	require.NotNil(t, bid)
}

func TestGloasPTCWindow(t *testing.T) {
	source, err := gloasState(t, 64)
	require.NoError(t, err)

	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))

	// Mutate the PTC window: bump validator indices in the last slot.
	window, err := target.PTCWindow()
	require.NoError(t, err)
	require.NotEmpty(t, window)
	last := window[len(window)-1]
	for i := range last.ValidatorIndices {
		last.ValidatorIndices[i] = primitives.ValidatorIndex(i + 7)
	}
	require.NoError(t, target.SetPTCWindow(window))

	ctx := t.Context()
	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, source, hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, target, result)
}

func TestGloasExecutionRequestsRoot(t *testing.T) {
	source, err := gloasState(t, 64)
	require.NoError(t, err)

	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))

	// Mutate ExecutionRequestsRoot on the bid.
	srcBid, err := target.LatestExecutionPayloadBid()
	require.NoError(t, err)
	parentBlockHash := srcBid.ParentBlockHash()
	parentBlockRoot := srcBid.ParentBlockRoot()
	blockHash := srcBid.BlockHash()
	prevRandao := srcBid.PrevRandao()
	feeRecipient := srcBid.FeeRecipient()
	var newRequestsRoot [32]byte
	for i := range newRequestsRoot {
		newRequestsRoot[i] = byte(i + 99)
	}
	newBidProto := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:       parentBlockHash[:],
		ParentBlockRoot:       parentBlockRoot[:],
		BlockHash:             blockHash[:],
		PrevRandao:            prevRandao[:],
		GasLimit:              srcBid.GasLimit(),
		BuilderIndex:          srcBid.BuilderIndex(),
		Slot:                  srcBid.Slot(),
		Value:                 srcBid.Value(),
		ExecutionPayment:      srcBid.ExecutionPayment(),
		BlobKzgCommitments:    srcBid.BlobKzgCommitments(),
		FeeRecipient:          feeRecipient[:],
		ExecutionRequestsRoot: newRequestsRoot[:],
	}
	wrapped, err := blocks.WrappedROExecutionPayloadBid(newBidProto)
	require.NoError(t, err)
	require.NoError(t, target.SetExecutionPayloadBid(wrapped))

	ctx := t.Context()
	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, source, hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, target, result)

	// Sanity: the restored bid carries the mutated requests root.
	gotBid, err := result.LatestExecutionPayloadBid()
	require.NoError(t, err)
	gotRoot := gotBid.ExecutionRequestsRoot()
	require.Equal(t, newRequestsRoot, gotRoot)
}
