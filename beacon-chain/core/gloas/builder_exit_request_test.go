package gloas

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func activeBuilder(t *testing.T, execAddr []byte) (*ethpb.Builder, bls.SecretKey) {
	t.Helper()
	sk, err := bls.RandKey()
	require.NoError(t, err)
	return &ethpb.Builder{
		Pubkey:            sk.PublicKey().Marshal(),
		Version:           []byte{params.BeaconConfig().BuilderWithdrawalPrefixByte},
		ExecutionAddress:  execAddr,
		Balance:           100,
		WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
	}, sk
}

func TestProcessBuilderExitRequest_InitiatesExit(t *testing.T) {
	addr := bytes.Repeat([]byte{0x44}, 20)
	builder, _ := activeBuilder(t, addr)
	st := newGloasState(t, nil, []*ethpb.Builder{builder})

	req := &enginev1.BuilderExitRequest{SourceAddress: addr, Pubkey: builder.Pubkey}
	require.NoError(t, processBuilderExitRequest(st, req))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(builder.Pubkey))
	require.Equal(t, true, ok)
	got, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MinBuilderWithdrawabilityDelay, got.WithdrawableEpoch)
}

func TestProcessBuilderExitRequest_UnknownPubkeyNoOp(t *testing.T) {
	st := newGloasState(t, nil, nil)
	req := &enginev1.BuilderExitRequest{SourceAddress: bytes.Repeat([]byte{0x44}, 20), Pubkey: make([]byte, 48)}
	require.NoError(t, processBuilderExitRequest(st, req))
}

func TestProcessBuilderExitRequest_WrongSourceAddressNoOp(t *testing.T) {
	addr := bytes.Repeat([]byte{0x44}, 20)
	builder, _ := activeBuilder(t, addr)
	st := newGloasState(t, nil, []*ethpb.Builder{builder})

	req := &enginev1.BuilderExitRequest{SourceAddress: bytes.Repeat([]byte{0x55}, 20), Pubkey: builder.Pubkey}
	require.NoError(t, processBuilderExitRequest(st, req))

	idx, _ := st.BuilderIndexByPubkey(toBytes48(builder.Pubkey))
	got, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().FarFutureEpoch, got.WithdrawableEpoch)
}
