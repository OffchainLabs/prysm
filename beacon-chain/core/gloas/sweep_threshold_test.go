package gloas

import (
	"bytes"
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// sweepThresholdSourceAddress is the execution address embedded in the test validator's
// withdrawal credentials, and therefore the only address allowed to set its threshold.
var sweepThresholdSourceAddress = bytes.Repeat([]byte{0x77}, 20)

func compoundingCredentials(addr []byte) []byte {
	creds := make([]byte, 32)
	creds[0] = params.BeaconConfig().CompoundingWithdrawalPrefixByte
	copy(creds[12:], addr)
	return creds
}

func eth1Credentials(addr []byte) []byte {
	creds := make([]byte, 32)
	creds[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
	copy(creds[12:], addr)
	return creds
}

// newSweepThresholdState builds a single-validator Gloas state with the given balance and
// a starting threshold, and returns the state plus the validator's pubkey.
func newSweepThresholdState(t *testing.T, creds []byte, balance, threshold uint64) (state.BeaconState, []byte) {
	t.Helper()

	sk, err := bls.RandKey()
	require.NoError(t, err)
	pubkey := sk.PublicKey().Marshal()

	st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
		Validators: []*ethpb.Validator{{
			PublicKey:             pubkey,
			WithdrawalCredentials: creds,
			EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalanceElectra,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
		}},
		Balances:                 []uint64{balance},
		ValidatorSweepThresholds: []uint64{threshold},
		FinalizedCheckpoint:      &ethpb.Checkpoint{Root: make([]byte, 32)},
	})
	require.NoError(t, err)

	return st, pubkey
}

func thresholdOf(t *testing.T, st state.BeaconState) uint64 {
	t.Helper()
	got, err := st.ValidatorSweepThreshold(0)
	require.NoError(t, err)
	return got
}

func TestProcessSetSweepThresholdRequest_SetsThreshold(t *testing.T) {
	// 33 ETH balance, request a 64 ETH threshold: accepted.
	balance := 33 * params.BeaconConfig().EffectiveBalanceIncrement
	want := 64 * params.BeaconConfig().EffectiveBalanceIncrement

	st, pubkey := newSweepThresholdState(t, compoundingCredentials(sweepThresholdSourceAddress), balance, params.BeaconConfig().MaxEffectiveBalanceElectra)

	req := &enginev1.SetSweepThresholdRequest{
		SourceAddress:   sweepThresholdSourceAddress,
		ValidatorPubkey: pubkey,
		Threshold:       want,
	}
	require.NoError(t, ProcessSetSweepThresholdRequests(context.Background(), st, []*enginev1.SetSweepThresholdRequest{req}))
	require.Equal(t, want, thresholdOf(t, st))
}

func TestProcessSetSweepThresholdRequest_Rejections(t *testing.T) {
	cfg := params.BeaconConfig()
	increment := cfg.EffectiveBalanceIncrement
	start := cfg.MaxEffectiveBalanceElectra
	balance := 33 * increment

	tests := []struct {
		name      string
		creds     []byte
		threshold uint64
		// mutate optionally adjusts the state before the request is processed.
		mutate func(t *testing.T, st state.BeaconState)
		// wrongPubkey sends the request for a pubkey that is not in the registry.
		wrongPubkey bool
		// sourceAddress overrides the request's source address.
		sourceAddress []byte
	}{
		{
			name:        "unknown pubkey",
			creds:       compoundingCredentials(sweepThresholdSourceAddress),
			threshold:   64 * increment,
			wrongPubkey: true,
		},
		{
			name:      "not compounding",
			creds:     eth1Credentials(sweepThresholdSourceAddress),
			threshold: 64 * increment,
		},
		{
			name:          "wrong source address",
			creds:         compoundingCredentials(sweepThresholdSourceAddress),
			threshold:     64 * increment,
			sourceAddress: bytes.Repeat([]byte{0x11}, 20),
		},
		{
			name:      "validator is exiting",
			creds:     compoundingCredentials(sweepThresholdSourceAddress),
			threshold: 64 * increment,
			mutate: func(t *testing.T, st state.BeaconState) {
				val, err := st.ValidatorAtIndex(0)
				require.NoError(t, err)
				val.ExitEpoch = 42
				require.NoError(t, st.UpdateValidatorAtIndex(0, val))
			},
		},
		{
			// A threshold below the current balance would let the validator sweep out
			// immediately, bypassing the partial withdrawal queue.
			name:      "threshold below current balance",
			creds:     compoundingCredentials(sweepThresholdSourceAddress),
			threshold: 32 * increment,
		},
		{
			name:      "threshold not a multiple of the increment",
			creds:     compoundingCredentials(sweepThresholdSourceAddress),
			threshold: 64*increment + 1,
		},
		{
			name:      "threshold below MIN_SWEEP_THRESHOLD",
			creds:     compoundingCredentials(sweepThresholdSourceAddress),
			threshold: cfg.MinActivationBalance,
			mutate: func(t *testing.T, st state.BeaconState) {
				// Drop the balance so the "below balance" rule does not mask this one.
				require.NoError(t, st.UpdateBalancesAtIndex(0, 0))
			},
		},
		{
			name:      "threshold above MAX_EFFECTIVE_BALANCE_ELECTRA",
			creds:     compoundingCredentials(sweepThresholdSourceAddress),
			threshold: cfg.MaxEffectiveBalanceElectra + increment,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, pubkey := newSweepThresholdState(t, tt.creds, balance, start)
			if tt.mutate != nil {
				tt.mutate(t, st)
			}
			if tt.wrongPubkey {
				sk, err := bls.RandKey()
				require.NoError(t, err)
				pubkey = sk.PublicKey().Marshal()
			}
			source := sweepThresholdSourceAddress
			if tt.sourceAddress != nil {
				source = tt.sourceAddress
			}

			req := &enginev1.SetSweepThresholdRequest{
				SourceAddress:   source,
				ValidatorPubkey: pubkey,
				Threshold:       tt.threshold,
			}
			require.NoError(t, ProcessSetSweepThresholdRequests(context.Background(), st, []*enginev1.SetSweepThresholdRequest{req}))
			require.Equal(t, start, thresholdOf(t, st))
		})
	}
}

func TestProcessSetSweepThresholdRequest_MinSweepThresholdAccepted(t *testing.T) {
	cfg := params.BeaconConfig()
	// MIN_SWEEP_THRESHOLD is exactly 33 ETH and must be accepted.
	require.Equal(t, cfg.MinActivationBalance+cfg.EffectiveBalanceIncrement, cfg.MinSweepThreshold())

	st, pubkey := newSweepThresholdState(t, compoundingCredentials(sweepThresholdSourceAddress), 0, cfg.MaxEffectiveBalanceElectra)
	req := &enginev1.SetSweepThresholdRequest{
		SourceAddress:   sweepThresholdSourceAddress,
		ValidatorPubkey: pubkey,
		Threshold:       cfg.MinSweepThreshold(),
	}
	require.NoError(t, ProcessSetSweepThresholdRequests(context.Background(), st, []*enginev1.SetSweepThresholdRequest{req}))
	require.Equal(t, cfg.MinSweepThreshold(), thresholdOf(t, st))
}

func TestProcessSetSweepThresholdRequest_UnchangedThresholdIsNoOp(t *testing.T) {
	cfg := params.BeaconConfig()
	st, pubkey := newSweepThresholdState(t, compoundingCredentials(sweepThresholdSourceAddress), 0, cfg.MaxEffectiveBalanceElectra)

	req := &enginev1.SetSweepThresholdRequest{
		SourceAddress:   sweepThresholdSourceAddress,
		ValidatorPubkey: pubkey,
		Threshold:       cfg.MaxEffectiveBalanceElectra,
	}
	require.NoError(t, ProcessSetSweepThresholdRequests(context.Background(), st, []*enginev1.SetSweepThresholdRequest{req}))
	require.Equal(t, cfg.MaxEffectiveBalanceElectra, thresholdOf(t, st))
}

func TestProcessSetSweepThresholdRequest_NilRequestErrors(t *testing.T) {
	st, _ := newSweepThresholdState(t, compoundingCredentials(sweepThresholdSourceAddress), 0, 0)
	err := ProcessSetSweepThresholdRequests(context.Background(), st, []*enginev1.SetSweepThresholdRequest{nil})
	require.ErrorContains(t, "nil set sweep threshold request", err)
}
