package verification

import (
	"testing"
	"time"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func TestHeaderVerifier_VerifyBuilderNotSlashedInactive(t *testing.T) {
	st, _ := util.DeterministicGenesisState(t, 3)
	val, err := st.ValidatorAtIndex(1)
	require.NoError(t, err)
	val.Slashed = true
	require.NoError(t, st.UpdateValidatorAtIndex(1, val))

	val, err = st.ValidatorAtIndex(2)
	require.NoError(t, err)
	val.ExitEpoch = 0
	require.NoError(t, st.UpdateValidatorAtIndex(2, val))

	now := time.Now()
	genesis := now.Add(-1 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	clock := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time { return now }))
	init := Initializer{shared: &sharedResources{clock: clock}}

	t.Run("not slashed and active", func(t *testing.T) {
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				BuilderIndex: 0,
			},
		}, st, init)
		require.NoError(t, h.VerifyBuilderActiveNotSlashed())
		require.Equal(t, true, h.results.executed(RequireBuilderActiveNotSlashed))
		require.NoError(t, h.results.result(RequireBuilderActiveNotSlashed))
	})

	t.Run("slashed", func(t *testing.T) {
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				BuilderIndex: 1,
			},
		}, st, init)
		require.ErrorIs(t, h.VerifyBuilderActiveNotSlashed(), ErrBuilderSlashed)
		require.Equal(t, true, h.results.executed(RequireBuilderActiveNotSlashed))
		require.Equal(t, ErrBuilderSlashed, h.results.result(RequireBuilderActiveNotSlashed))
	})

	t.Run("inactive", func(t *testing.T) {
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				BuilderIndex: 2,
			},
		}, st, init)
		require.ErrorIs(t, h.VerifyBuilderActiveNotSlashed(), ErrBuilderInactive)
		require.Equal(t, true, h.results.executed(RequireBuilderActiveNotSlashed))
		require.Equal(t, ErrBuilderInactive, h.results.result(RequireBuilderActiveNotSlashed))
	})
}

func TestHeaderVerifier_VerifyBuilderSufficientBalance(t *testing.T) {
	st, _ := util.DeterministicGenesisState(t, 1)
	mbb := params.BeaconConfig().MinBuilderBalance
	require.NoError(t, st.SetBalances([]uint64{mbb, mbb + 1}))

	init := Initializer{shared: &sharedResources{}}

	t.Run("happy case", func(t *testing.T) {
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				BuilderIndex: 1,
				Value:        1,
			},
		}, st, init)
		require.NoError(t, h.VerifyBuilderSufficientBalance())
		require.Equal(t, true, h.results.executed(RequireBuilderSufficientBalance))
		require.NoError(t, h.results.result(RequireBuilderSufficientBalance))
	})

	t.Run("insufficient balance", func(t *testing.T) {
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				BuilderIndex: 0,
				Value:        1,
			},
		}, st, init)
		require.ErrorIs(t, h.VerifyBuilderSufficientBalance(), ErrBuilderInsufficientBalance)
		require.Equal(t, true, h.results.executed(RequireBuilderSufficientBalance))
		require.Equal(t, ErrBuilderInsufficientBalance, h.results.result(RequireBuilderSufficientBalance))
	})
}

func TestHeaderVerifier_VerifyCurrentOrNextSlot(t *testing.T) {
	now := time.Now()
	genesis := now.Add(-1 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	clock := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time { return now }))

	init := Initializer{shared: &sharedResources{clock: clock}}

	t.Run("current slot", func(t *testing.T) {
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot: 1,
			},
		}, nil, init)
		require.NoError(t, h.VerifyCurrentOrNextSlot())
		require.Equal(t, true, h.results.executed(RequireCurrentOrNextSlot))
		require.NoError(t, h.results.result(RequireCurrentOrNextSlot))
	})

	t.Run("next slot", func(t *testing.T) {
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot: 2,
			},
		}, nil, init)
		require.NoError(t, h.VerifyCurrentOrNextSlot())
		require.Equal(t, true, h.results.executed(RequireCurrentOrNextSlot))
		require.NoError(t, h.results.result(RequireCurrentOrNextSlot))
	})

	t.Run("incorrect slot", func(t *testing.T) {
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot: 3,
			},
		}, nil, init)
		require.ErrorIs(t, h.VerifyCurrentOrNextSlot(), ErrIncorrectPayloadHeaderSlot)
		require.Equal(t, true, h.results.executed(RequireCurrentOrNextSlot))
		require.Equal(t, ErrIncorrectPayloadHeaderSlot, h.results.result(RequireCurrentOrNextSlot))
	})
}

func TestHeaderVerifier_VerifyParentBlockHashSeen(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{}, nil, init)
		require.NoError(t, h.VerifyParentBlockHashSeen(
			func(_ [32]byte) bool {
				return true
			},
		))
		require.Equal(t, true, h.results.executed(RequireKnownParentBlockHash))
		require.NoError(t, h.results.result(RequireKnownParentBlockHash))
	})

	t.Run("unknown parent hash", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{}, nil, init)
		require.ErrorIs(t, h.VerifyParentBlockHashSeen(
			func(_ [32]byte) bool {
				return false
			},
		), ErrUnknownParentBlockHash)
		require.Equal(t, true, h.results.executed(RequireKnownParentBlockHash))
		require.Equal(t, ErrUnknownParentBlockHash, h.results.result(RequireKnownParentBlockHash))
	})
}

func TestHeaderVerifier_VerifyParentBlockRootSeen(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{}, nil, init)
		require.NoError(t, h.VerifyParentBlockRootSeen(
			func(_ [32]byte) bool {
				return true
			},
		))
		require.Equal(t, true, h.results.executed(RequireKnownParentBlockRoot))
		require.NoError(t, h.results.result(RequireKnownParentBlockRoot))
	})

	t.Run("unknown parent root", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		h := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{}, nil, init)
		require.ErrorIs(t, h.VerifyParentBlockRootSeen(
			func(_ [32]byte) bool {
				return false
			},
		), ErrUnknownParentBlockRoot)
		require.Equal(t, true, h.results.executed(RequireKnownParentBlockRoot))
		require.Equal(t, ErrUnknownParentBlockRoot, h.results.result(RequireKnownParentBlockRoot))
	})
}

func TestHeaderVerifier_VerifySignature(t *testing.T) {
	_, secretKeys, err := util.DeterministicDepositsAndKeys(2)
	require.NoError(t, err)

	st, err := state_native.InitializeFromProtoEpbs(&ethpb.BeaconStateEPBS{
		Validators: []*ethpb.Validator{{PublicKey: secretKeys[0].PublicKey().Marshal()},
			{PublicKey: secretKeys[1].PublicKey().Marshal()}},
		Fork: &ethpb.Fork{
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
		},
	})
	require.NoError(t, err)

	t.Run("valid signature", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		sh := util.HydrateSignedExecutionPayloadHeader(&enginev1.SignedExecutionPayloadHeader{})
		h := sh.Message

		signedBytes, err := signing.ComputeDomainAndSign(
			st,
			slots.ToEpoch(h.Slot),
			h,
			params.BeaconConfig().DomainBeaconBuilder,
			secretKeys[0],
		)
		require.NoError(t, err)
		sig, err := bls.SignatureFromBytes(signedBytes)
		require.NoError(t, err)
		pa := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{
			Message:   h,
			Signature: sig.Marshal(),
		}, st, init)

		require.NoError(t, pa.VerifySignature())
		require.Equal(t, true, pa.results.executed(RequireSignatureValid))
		require.NoError(t, pa.results.result(RequireSignatureValid))
	})

	t.Run("invalid signature", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		sh := util.HydrateSignedExecutionPayloadHeader(&enginev1.SignedExecutionPayloadHeader{})
		h := sh.Message
		signedBytes, err := signing.ComputeDomainAndSign(
			st,
			slots.ToEpoch(h.Slot),
			h,
			params.BeaconConfig().DomainBeaconBuilder,
			secretKeys[1],
		)
		require.NoError(t, err)
		sig, err := bls.SignatureFromBytes(signedBytes)
		require.NoError(t, err)
		pa := newExecutionPayloadHeader(t, &enginev1.SignedExecutionPayloadHeader{
			Message:   h,
			Signature: sig.Marshal(),
		}, st, init)

		require.ErrorIs(t, pa.VerifySignature(), signing.ErrSigFailedToVerify)
		require.Equal(t, true, pa.results.executed(RequireSignatureValid))
		require.Equal(t, signing.ErrSigFailedToVerify, pa.results.result(RequireSignatureValid))
	})
}

func newExecutionPayloadHeader(t *testing.T, h *enginev1.SignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, init Initializer) *HeaderVerifier {
	h = util.HydrateSignedExecutionPayloadHeader(h)
	ro, err := blocks.WrappedROSignedExecutionPayloadHeader(h)
	require.NoError(t, err)
	return init.NewHeaderVerifier(ro, st, GossipExecutionPayloadHeaderRequirements)
}
