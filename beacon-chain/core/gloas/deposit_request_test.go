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
		err := processDepositRequests(t.Context(), st, []*enginev1.DepositRequest{})
		require.NoError(t, err)
	})

	t.Run("nil request errors", func(t *testing.T) {
		err := processDepositRequests(t.Context(), st, []*enginev1.DepositRequest{nil})
		require.ErrorContains(t, "nil deposit request", err)
	})
}

func TestProcessDepositRequest_BuilderDepositAddsBuilder(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)

	cred := builderWithdrawalCredentials()
	pd := stateTesting.GeneratePendingDeposit(t, sk, 1234, cred, 0)
	req := depositRequestFromPending(pd, 1)

	st := newGloasState(t, nil, nil)
	err = processDepositRequest(st, req)
	require.NoError(t, err)

	idx, ok := st.BuilderIndexByPubkey(toBytes48(req.Pubkey))
	require.Equal(t, true, ok)

	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.NotNil(t, builder)
	require.DeepEqual(t, req.Pubkey, builder.Pubkey)
	require.DeepEqual(t, []byte{cred[0]}, builder.Version)
	require.DeepEqual(t, cred[12:], builder.ExecutionAddress)
	require.Equal(t, uint64(1234), uint64(builder.Balance))
	require.Equal(t, params.BeaconConfig().FarFutureEpoch, builder.WithdrawableEpoch)

	pending, err := st.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 0, len(pending))
}

func TestProcessDepositRequest_ExistingBuilderIncreasesBalance(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)

	pubkey := sk.PublicKey().Marshal()
	builders := []*ethpb.Builder{
		{
			Pubkey:            pubkey,
			Version:           []byte{0},
			ExecutionAddress:  bytes.Repeat([]byte{0x11}, 20),
			Balance:           5,
			WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
		},
	}
	st := newGloasState(t, nil, builders)

	cred := validatorWithdrawalCredentials()
	pd := stateTesting.GeneratePendingDeposit(t, sk, 200, cred, 0)
	req := depositRequestFromPending(pd, 9)

	err = processDepositRequest(st, req)
	require.NoError(t, err)

	idx, ok := st.BuilderIndexByPubkey(toBytes48(pubkey))
	require.Equal(t, true, ok)
	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(205), uint64(builder.Balance))

	pending, err := st.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 0, len(pending))
}

func TestApplyDepositForBuilder_InvalidSignatureIgnoresDeposit(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)

	cred := builderWithdrawalCredentials()
	st := newGloasState(t, nil, nil)
	err = applyDepositForBuilder(st, sk.PublicKey().Marshal(), cred[:], 100, make([]byte, 96))
	require.NoError(t, err)

	_, ok := st.BuilderIndexByPubkey(toBytes48(sk.PublicKey().Marshal()))
	require.Equal(t, false, ok)
}

func newGloasState(t *testing.T, validators []*ethpb.Validator, builders []*ethpb.Builder) state.BeaconState {
	t.Helper()

	st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
		DepositRequestsStartIndex: params.BeaconConfig().UnsetDepositRequestsStartIndex,
		Validators:                validators,
		Balances:                  make([]uint64, len(validators)),
		PendingDeposits:           []*ethpb.PendingDeposit{},
		Builders:                  builders,
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

func validatorWithdrawalCredentials() [32]byte {
	var cred [32]byte
	cred[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
	copy(cred[12:], bytes.Repeat([]byte{0x33}, 20))
	return cred
}

func toBytes48(b []byte) [48]byte {
	var out [48]byte
	copy(out[:], b)
	return out
}
