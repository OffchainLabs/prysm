package gloas

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func signedBuilderDepositRequest(t *testing.T, sk bls.SecretKey, cred [32]byte, amount uint64) *enginev1.BuilderDepositRequest {
	t.Helper()
	pubkey := sk.PublicKey().Marshal()
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainBuilderDeposit, nil, nil)
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
		PublicKey:             pubkey,
		WithdrawalCredentials: cred[:],
		Amount:                amount,
	}, domain)
	require.NoError(t, err)
	return &enginev1.BuilderDepositRequest{
		Pubkey:                pubkey,
		WithdrawalCredentials: cred[:],
		Amount:                amount,
		Signature:             sk.Sign(root[:]).Marshal(),
	}
}

func TestProcessBuilderDepositRequest_NewBuilderValidSignature(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	cred := builderWithdrawalCredentials()
	req := signedBuilderDepositRequest(t, sk, cred, 1234)

	st := newGloasState(t, nil, nil)
	require.NoError(t, processBuilderDepositRequest(st, req))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(req.Pubkey))
	require.Equal(t, true, ok)
	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.DeepEqual(t, req.Pubkey, builder.Pubkey)
	require.DeepEqual(t, []byte{cred[0]}, builder.Version)
	require.DeepEqual(t, cred[12:], builder.ExecutionAddress)
	require.Equal(t, uint64(1234), uint64(builder.Balance))
	require.Equal(t, params.BeaconConfig().FarFutureEpoch, builder.WithdrawableEpoch)
}

func TestProcessBuilderDepositRequest_NewBuilderInvalidSignatureIgnored(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	cred := builderWithdrawalCredentials()
	req := signedBuilderDepositRequest(t, sk, cred, 1234)
	req.Signature = make([]byte, 96) // invalidate

	st := newGloasState(t, nil, nil)
	require.NoError(t, processBuilderDepositRequest(st, req))

	_, ok := st.BuilderIndexByPubkey(toBytes48(req.Pubkey))
	require.Equal(t, false, ok)
}

func TestProcessBuilderDepositRequest_ExistingBuilderTopsUpNoSignatureCheck(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	pubkey := sk.PublicKey().Marshal()
	builders := []*ethpb.Builder{{
		Pubkey:            pubkey,
		Version:           []byte{params.BeaconConfig().BuilderWithdrawalPrefixByte},
		ExecutionAddress:  bytes.Repeat([]byte{0x11}, 20),
		Balance:           5,
		WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
	}}
	st := newGloasState(t, nil, builders)

	req := &enginev1.BuilderDepositRequest{
		Pubkey:                pubkey,
		WithdrawalCredentials: builderWithdrawalCredentialsAt(0x99),
		Amount:                200,
		Signature:             make([]byte, 96), // not checked for top-up
	}
	require.NoError(t, processBuilderDepositRequest(st, req))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(pubkey))
	require.Equal(t, true, ok)
	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(205), uint64(builder.Balance))
}

func builderWithdrawalCredentialsAt(b byte) []byte {
	cred := make([]byte, 32)
	cred[0] = params.BeaconConfig().BuilderWithdrawalPrefixByte
	copy(cred[12:], bytes.Repeat([]byte{b}, 20))
	return cred
}
