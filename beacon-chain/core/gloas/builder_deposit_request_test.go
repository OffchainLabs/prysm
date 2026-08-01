package gloas

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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
	require.NoError(t, processBuilderDepositRequest(t.Context(), st, req, sigKnownValid))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(req.Pubkey))
	require.Equal(t, true, ok)
	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.DeepEqual(t, req.Pubkey, builder.Pubkey)
	require.DeepEqual(t, []byte{params.BeaconConfig().PayloadBuilderVersion}, builder.Version)
	require.DeepEqual(t, cred[12:], builder.ExecutionAddress)
	require.Equal(t, uint64(1234), uint64(builder.Balance))
	require.Equal(t, params.BeaconConfig().FarFutureEpoch, builder.WithdrawableEpoch)
}

func TestProcessBuilderDepositRequest_NewBuilderInvalidSignatureIgnored(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	cred := builderWithdrawalCredentials()
	req := signedBuilderDepositRequest(t, sk, cred, 1234)

	st := newGloasState(t, nil, nil)
	require.NoError(t, processBuilderDepositRequest(t.Context(), st, req, sigKnownInvalid))

	_, ok := st.BuilderIndexByPubkey(toBytes48(req.Pubkey))
	require.Equal(t, false, ok)
}

func TestProcessBuilderDepositRequest_NewBuilderNonBuilderPrefixIgnored(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	cred := builderWithdrawalCredentials()
	cred[0] = 0x01
	req := signedBuilderDepositRequest(t, sk, cred, 1234)

	st := newGloasState(t, nil, nil)
	require.NoError(t, processBuilderDepositRequest(t.Context(), st, req, sigKnownValid))

	_, ok := st.BuilderIndexByPubkey(toBytes48(req.Pubkey))
	require.Equal(t, false, ok)
}

func TestProcessBuilderDepositRequest_TopUpNonBuilderPrefixIgnored(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	pubkey := sk.PublicKey().Marshal()
	builders := []*ethpb.Builder{{
		Pubkey:            pubkey,
		Version:           []byte{params.BeaconConfig().PayloadBuilderVersion},
		ExecutionAddress:  bytes.Repeat([]byte{0x11}, 20),
		Balance:           5,
		WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
	}}
	st := newGloasState(t, nil, builders)

	cred := builderWithdrawalCredentials()
	cred[0] = 0x01
	req := &enginev1.BuilderDepositRequest{
		Pubkey:                pubkey,
		WithdrawalCredentials: cred[:],
		Amount:                200,
		Signature:             make([]byte, 96),
	}
	require.NoError(t, processBuilderDepositRequest(t.Context(), st, req, sigKnownValid))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(pubkey))
	require.Equal(t, true, ok)
	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(5), uint64(builder.Balance))
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
	require.NoError(t, processBuilderDepositRequest(t.Context(), st, req, sigUnverified))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(pubkey))
	require.Equal(t, true, ok)
	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(205), uint64(builder.Balance))
}

func TestProcessBuilderDepositRequests_BatchMixedValidInvalidAndTopUp(t *testing.T) {
	cred := builderWithdrawalCredentials()
	skValid, err := bls.RandKey()
	require.NoError(t, err)
	skInvalid, err := bls.RandKey()
	require.NoError(t, err)
	skExisting, err := bls.RandKey()
	require.NoError(t, err)
	existingPubkey := skExisting.PublicKey().Marshal()

	builders := []*ethpb.Builder{{
		Pubkey:            existingPubkey,
		Version:           []byte{params.BeaconConfig().BuilderWithdrawalPrefixByte},
		ExecutionAddress:  bytes.Repeat([]byte{0x11}, 20),
		Balance:           5,
		WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
	}}
	st := newGloasState(t, nil, builders)

	invalidReq := signedBuilderDepositRequest(t, skInvalid, cred, 1234)
	invalidReq.Signature = make([]byte, 96)
	reqs := []*enginev1.BuilderDepositRequest{
		signedBuilderDepositRequest(t, skValid, cred, 1000),
		invalidReq,
		{Pubkey: existingPubkey, WithdrawalCredentials: cred[:], Amount: 200, Signature: make([]byte, 96)},
	}
	require.NoError(t, ProcessBuilderDepositRequests(t.Context(), st, reqs))

	_, ok := st.BuilderIndexByPubkey(toBytes48(skValid.PublicKey().Marshal()))
	require.Equal(t, true, ok)
	_, ok = st.BuilderIndexByPubkey(toBytes48(skInvalid.PublicKey().Marshal()))
	require.Equal(t, false, ok)
	idx, ok := st.BuilderIndexByPubkey(toBytes48(existingPubkey))
	require.Equal(t, true, ok)
	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(205), uint64(builder.Balance))
}

func TestProcessBuilderDepositRequests_DuplicatePubkeyValidThenTopUp(t *testing.T) {
	cred := builderWithdrawalCredentials()
	sk, err := bls.RandKey()
	require.NoError(t, err)

	st := newGloasState(t, nil, nil)
	reqs := []*enginev1.BuilderDepositRequest{
		signedBuilderDepositRequest(t, sk, cred, 1000),
		// top-up: bad signature is fine
		{Pubkey: sk.PublicKey().Marshal(), WithdrawalCredentials: cred[:], Amount: 500, Signature: make([]byte, 96)},
	}
	require.NoError(t, ProcessBuilderDepositRequests(t.Context(), st, reqs))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(sk.PublicKey().Marshal()))
	require.Equal(t, true, ok)
	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(1500), uint64(builder.Balance))
}

func TestProcessBuilderDepositRequests_DuplicatePubkeyInvalidThenValidRegisters(t *testing.T) {
	cred := builderWithdrawalCredentials()
	sk, err := bls.RandKey()
	require.NoError(t, err)

	st := newGloasState(t, nil, nil)
	invalidReq := signedBuilderDepositRequest(t, sk, cred, 1000)
	invalidReq.Signature = make([]byte, 96)
	reqs := []*enginev1.BuilderDepositRequest{
		invalidReq,
		signedBuilderDepositRequest(t, sk, cred, 700),
	}
	require.NoError(t, ProcessBuilderDepositRequests(t.Context(), st, reqs))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(sk.PublicKey().Marshal()))
	require.Equal(t, true, ok)
	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(700), uint64(builder.Balance))
}

func TestProcessBuilderDepositRequests_EvictedPubkeyReverifiesSignature(t *testing.T) {
	cred := builderWithdrawalCredentials()
	skNew, err := bls.RandKey()
	require.NoError(t, err)
	skVictim, err := bls.RandKey()
	require.NoError(t, err)
	victimPubkey := skVictim.PublicKey().Marshal()

	builders := []*ethpb.Builder{{
		Pubkey:            victimPubkey,
		Version:           []byte{params.BeaconConfig().BuilderWithdrawalPrefixByte},
		ExecutionAddress:  bytes.Repeat([]byte{0x11}, 20),
		Balance:           0,
		WithdrawableEpoch: 0,
	}}
	st := newGloasState(t, nil, builders)

	victimReq := &enginev1.BuilderDepositRequest{
		Pubkey:                victimPubkey,
		WithdrawalCredentials: cred[:],
		Amount:                200,
		Signature:             make([]byte, 96), // garbage
	}
	reqs := []*enginev1.BuilderDepositRequest{
		signedBuilderDepositRequest(t, skNew, cred, 1000), // reuses index 0, evicts victim
		victimReq,
	}
	require.NoError(t, ProcessBuilderDepositRequests(t.Context(), st, reqs))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(skNew.PublicKey().Marshal()))
	require.Equal(t, true, ok)
	require.Equal(t, primitives.BuilderIndex(0), idx)

	_, ok = st.BuilderIndexByPubkey(toBytes48(victimPubkey))
	require.Equal(t, false, ok)
}

func TestProcessBuilderDepositRequests_EvictedPubkeyValidSignatureRegisters(t *testing.T) {
	cred := builderWithdrawalCredentials()
	skNew, err := bls.RandKey()
	require.NoError(t, err)
	skVictim, err := bls.RandKey()
	require.NoError(t, err)
	victimPubkey := skVictim.PublicKey().Marshal()

	builders := []*ethpb.Builder{{
		Pubkey:            victimPubkey,
		Version:           []byte{params.BeaconConfig().BuilderWithdrawalPrefixByte},
		ExecutionAddress:  bytes.Repeat([]byte{0x11}, 20),
		Balance:           0,
		WithdrawableEpoch: 0,
	}}
	st := newGloasState(t, nil, builders)

	reqs := []*enginev1.BuilderDepositRequest{
		signedBuilderDepositRequest(t, skNew, cred, 1000), // reuses index 0, evicts victim
		signedBuilderDepositRequest(t, skVictim, cred, 200),
	}
	require.NoError(t, ProcessBuilderDepositRequests(t.Context(), st, reqs))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(skNew.PublicKey().Marshal()))
	require.Equal(t, true, ok)
	require.Equal(t, primitives.BuilderIndex(0), idx)

	idx, ok = st.BuilderIndexByPubkey(toBytes48(victimPubkey))
	require.Equal(t, true, ok)
	require.Equal(t, primitives.BuilderIndex(1), idx)
	builder, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(200), uint64(builder.Balance))
}

func builderWithdrawalCredentialsAt(b byte) []byte {
	cred := make([]byte, 32)
	cred[0] = params.BeaconConfig().BuilderWithdrawalPrefixByte
	copy(cred[12:], bytes.Repeat([]byte{b}, 20))
	return cred
}
