package gloas

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	stateTesting "github.com/OffchainLabs/prysm/v7/beacon-chain/state/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProcessDepositRequests_EmptyAndNil(t *testing.T) {
	st := newGloasState(t, nil, nil)

	t.Run("empty requests continues", func(t *testing.T) {
		err := ProcessDepositRequests(t.Context(), st, []*enginev1.DepositRequest{})
		require.NoError(t, err)
	})

	t.Run("nil request errors", func(t *testing.T) {
		err := ProcessDepositRequests(t.Context(), st, []*enginev1.DepositRequest{nil})
		require.ErrorContains(t, "nil deposit request", err)
	})
}

// [Modified in Gloas:EIP8282] All deposit requests, including those with a
// builder withdrawal credential, are queued as pending deposits; builder
// onboarding happens only via BuilderDepositRequest.
func TestProcessDepositRequest_QueuesPendingDeposit(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)

	cred := builderWithdrawalCredentials()
	pd := stateTesting.GeneratePendingDeposit(t, sk, 1234, cred, 0)
	req := depositRequestFromPending(pd, 1)

	st := newGloasState(t, nil, nil)
	require.NoError(t, processDepositRequest(st, req))

	_, ok := st.BuilderIndexByPubkey(toBytes48(req.Pubkey))
	require.Equal(t, false, ok)

	pending, err := st.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 1, len(pending))
	require.DeepEqual(t, req.Pubkey, pending[0].PublicKey)
	require.DeepEqual(t, req.WithdrawalCredentials, pending[0].WithdrawalCredentials)
	require.Equal(t, req.Amount, pending[0].Amount)
}

func newGloasState(t *testing.T, validators []*ethpb.Validator, builders []*ethpb.Builder) state.BeaconState {
	t.Helper()

	st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
		DepositRequestsStartIndex: params.BeaconConfig().UnsetDepositRequestsStartIndex,
		Validators:                validators,
		Balances:                  make([]uint64, len(validators)),
		PendingDeposits:           []*ethpb.PendingDeposit{},
		Builders:                  builders,
		FinalizedCheckpoint:       &ethpb.Checkpoint{Epoch: 1, Root: make([]byte, 32)},
	})
	require.NoError(t, err)

	return st
}

func depositRequestFromPending(pd *ethpb.PendingDeposit, index uint64) *enginev1.DepositRequest {
	return &enginev1.DepositRequest{
		Pubkey:                pd.PublicKey,
		WithdrawalCredentials: pd.WithdrawalCredentials,
		Amount:                pd.Amount,
		Signature:             pd.Signature,
		Index:                 index,
	}
}

func builderWithdrawalCredentials() [32]byte {
	var cred [32]byte
	cred[0] = params.BeaconConfig().BuilderWithdrawalPrefixByte
	copy(cred[12:], bytes.Repeat([]byte{0x22}, 20))
	return cred
}

func toBytes48(b []byte) [48]byte {
	var out [48]byte
	copy(out[:], b)
	return out
}
