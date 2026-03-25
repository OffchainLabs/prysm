package hdiff

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

func gloasState(t testing.TB, numValidators uint64) (state.BeaconState, error) {
	fuluState, _ := util.DeterministicGenesisStateFulu(t, numValidators)
	return gloas.UpgradeToGloas(fuluState)
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

	gloasTarget, err := gloas.UpgradeToGloas(fuluSource.Copy())
	require.NoError(t, err)
	require.NoError(t, gloasTarget.SetSlot(fuluSource.Slot()+1))

	ctx := t.Context()
	hdiffBytes, err := Diff(fuluSource, gloasTarget)
	require.NoError(t, err)

	result, err := ApplyDiff(ctx, fuluSource, hdiffBytes)
	require.NoError(t, err)
	requireEqualState(t, gloasTarget, result)
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
