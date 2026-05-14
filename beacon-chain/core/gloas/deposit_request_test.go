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
		err := processDepositRequests(t.Context(), st, []*enginev1.DepositRequest{}, nil)
		require.NoError(t, err)
	})

	t.Run("nil request errors", func(t *testing.T) {
		err := processDepositRequests(t.Context(), st, []*enginev1.DepositRequest{nil}, nil)
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

func TestProcessDepositRequest_BuilderDepositWithExistingPendingDepositStaysPending(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)

	validatorCred := validatorWithdrawalCredentials()
	builderCred := builderWithdrawalCredentials()
	existingPending := stateTesting.GeneratePendingDeposit(t, sk, 1234, validatorCred, 0)
	req := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, sk, 200, builderCred, 1), 9)

	st := newGloasState(t, nil, nil)
	require.NoError(t, st.SetPendingDeposits([]*ethpb.PendingDeposit{existingPending}))

	err = processDepositRequest(st, req)
	require.NoError(t, err)

	_, ok := st.BuilderIndexByPubkey(toBytes48(req.Pubkey))
	require.Equal(t, false, ok)

	pending, err := st.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 2, len(pending))
	require.DeepEqual(t, existingPending.PublicKey, pending[0].PublicKey)
	require.DeepEqual(t, req.Pubkey, pending[1].PublicKey)
	require.DeepEqual(t, req.WithdrawalCredentials, pending[1].WithdrawalCredentials)
	require.Equal(t, req.Amount, pending[1].Amount)
}

func TestApplyDepositForBuilder_InvalidSignatureIgnoresDeposit(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)

	cred := builderWithdrawalCredentials()
	st := newGloasState(t, nil, nil)
	err = applyDepositForNewBuilder(st, sk.PublicKey().Marshal(), cred[:], 100, make([]byte, 96))
	require.NoError(t, err)

	_, ok := st.BuilderIndexByPubkey(toBytes48(sk.PublicKey().Marshal()))
	require.Equal(t, false, ok)
}

func TestProcessDepositRequests_BatchVerifyMultipleNewBuilders(t *testing.T) {
	cred := builderWithdrawalCredentials()
	const n = 4
	keys := make([]bls.SecretKey, n)
	reqs := make([]*enginev1.DepositRequest, n)
	for i := range n {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		keys[i] = sk
		pd := stateTesting.GeneratePendingDeposit(t, sk, uint64(1000+i), cred, 0)
		reqs[i] = depositRequestFromPending(pd, uint64(i))
	}

	st := newGloasState(t, nil, nil)
	require.NoError(t, processDepositRequests(t.Context(), st, reqs, nil))

	for i := range n {
		idx, ok := st.BuilderIndexByPubkey(toBytes48(keys[i].PublicKey().Marshal()))
		require.Equal(t, true, ok)
		b, err := st.Builder(idx)
		require.NoError(t, err)
		require.Equal(t, uint64(1000+i), uint64(b.Balance))
	}
	pending, err := st.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 0, len(pending))
}

func TestProcessDepositRequests_BatchFallback_OnlyInvalidDropped(t *testing.T) {
	cred := builderWithdrawalCredentials()

	skGood, err := bls.RandKey()
	require.NoError(t, err)
	good := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, skGood, 5000, cred, 0), 0)

	skBad, err := bls.RandKey()
	require.NoError(t, err)
	bad := &enginev1.DepositRequest{
		Pubkey:                skBad.PublicKey().Marshal(),
		WithdrawalCredentials: cred[:],
		Amount:                7777,
		Signature:             make([]byte, 96),
		Index:                 1,
	}

	st := newGloasState(t, nil, nil)
	require.NoError(t, processDepositRequests(t.Context(), st, []*enginev1.DepositRequest{good, bad}, nil))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(good.Pubkey))
	require.Equal(t, true, ok)
	b, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(5000), uint64(b.Balance))

	_, ok = st.BuilderIndexByPubkey(toBytes48(bad.Pubkey))
	require.Equal(t, false, ok)

	pending, err := st.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 0, len(pending))
}

func TestProcessDepositRequests_BatchFallback_AllInvalidDropped(t *testing.T) {
	cred := builderWithdrawalCredentials()

	mk := func(idx uint64) *enginev1.DepositRequest {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		return &enginev1.DepositRequest{
			Pubkey:                sk.PublicKey().Marshal(),
			WithdrawalCredentials: cred[:],
			Amount:                1000,
			Signature:             make([]byte, 96),
			Index:                 idx,
		}
	}
	reqs := []*enginev1.DepositRequest{mk(0), mk(1), mk(2)}

	st := newGloasState(t, nil, nil)
	require.NoError(t, processDepositRequests(t.Context(), st, reqs, nil))

	for _, r := range reqs {
		_, ok := st.BuilderIndexByPubkey(toBytes48(r.Pubkey))
		require.Equal(t, false, ok)
	}
	pending, err := st.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 0, len(pending))
}

func TestProcessDepositRequests_DupPubkey_RegistrationThenTopUp(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	cred := builderWithdrawalCredentials()
	register := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, sk, 1000, cred, 0), 0)
	topUpCred := validatorWithdrawalCredentials()
	topUp := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, sk, 250, topUpCred, 0), 1)

	st := newGloasState(t, nil, nil)
	require.NoError(t, processDepositRequests(t.Context(), st, []*enginev1.DepositRequest{register, topUp}, nil))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(sk.PublicKey().Marshal()))
	require.Equal(t, true, ok)
	b, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(1250), uint64(b.Balance))
}

func TestProcessDepositRequests_DupPubkey_FailedRegistrationThenTopUpRetries(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	cred := builderWithdrawalCredentials()

	first := &enginev1.DepositRequest{
		Pubkey:                sk.PublicKey().Marshal(),
		WithdrawalCredentials: cred[:],
		Amount:                1234,
		Signature:             make([]byte, 96),
		Index:                 0,
	}
	second := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, sk, 999, cred, 0), 1)

	st := newGloasState(t, nil, nil)
	require.NoError(t, processDepositRequests(t.Context(), st, []*enginev1.DepositRequest{first, second}, nil))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(sk.PublicKey().Marshal()))
	require.Equal(t, true, ok)
	b, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(999), uint64(b.Balance))
}

func TestProcessDepositRequests_DupPubkey_ValidatorThenBuilderStaysPending(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	valCred := validatorWithdrawalCredentials()
	bldCred := builderWithdrawalCredentials()
	first := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, sk, 100, valCred, 0), 0)
	second := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, sk, 200, bldCred, 0), 1)

	st := newGloasState(t, nil, nil)
	require.NoError(t, processDepositRequests(t.Context(), st, []*enginev1.DepositRequest{first, second}, nil))

	_, ok := st.BuilderIndexByPubkey(toBytes48(sk.PublicKey().Marshal()))
	require.Equal(t, false, ok)
	pending, err := st.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 2, len(pending))
	require.DeepEqual(t, first.Pubkey, pending[0].PublicKey)
	require.DeepEqual(t, valCred[:], pending[0].WithdrawalCredentials)
	require.DeepEqual(t, second.Pubkey, pending[1].PublicKey)
	require.DeepEqual(t, bldCred[:], pending[1].WithdrawalCredentials)
}

func TestProcessDepositRequests_MixedClassifications(t *testing.T) {
	skExisting, err := bls.RandKey()
	require.NoError(t, err)
	existingPubkey := skExisting.PublicKey().Marshal()
	existingBuilders := []*ethpb.Builder{
		{
			Pubkey:            existingPubkey,
			Version:           []byte{0},
			ExecutionAddress:  bytes.Repeat([]byte{0xaa}, 20),
			Balance:           42,
			WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
		},
	}
	st := newGloasState(t, nil, existingBuilders)

	bldCred := builderWithdrawalCredentials()
	valCred := validatorWithdrawalCredentials()

	topUp := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, skExisting, 8, valCred, 0), 0)

	skNewA, err := bls.RandKey()
	require.NoError(t, err)
	newA := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, skNewA, 1111, bldCred, 0), 1)

	skVal, err := bls.RandKey()
	require.NoError(t, err)
	valDep := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, skVal, 32, valCred, 0), 2)

	skNewB, err := bls.RandKey()
	require.NoError(t, err)
	newB := depositRequestFromPending(stateTesting.GeneratePendingDeposit(t, skNewB, 2222, bldCred, 0), 3)

	require.NoError(t, processDepositRequests(t.Context(), st, []*enginev1.DepositRequest{topUp, newA, valDep, newB}, nil))

	idx, ok := st.BuilderIndexByPubkey(toBytes48(existingPubkey))
	require.Equal(t, true, ok)
	bExisting, err := st.Builder(idx)
	require.NoError(t, err)
	require.Equal(t, uint64(50), uint64(bExisting.Balance))

	idxA, ok := st.BuilderIndexByPubkey(toBytes48(newA.Pubkey))
	require.Equal(t, true, ok)
	bA, err := st.Builder(idxA)
	require.NoError(t, err)
	require.Equal(t, uint64(1111), uint64(bA.Balance))

	idxB, ok := st.BuilderIndexByPubkey(toBytes48(newB.Pubkey))
	require.Equal(t, true, ok)
	bB, err := st.Builder(idxB)
	require.NoError(t, err)
	require.Equal(t, uint64(2222), uint64(bB.Balance))

	pending, err := st.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 1, len(pending))
	require.DeepEqual(t, valDep.Pubkey, pending[0].PublicKey)
	require.Equal(t, valDep.Amount, pending[0].Amount)
}

func TestProcessDepositRequests_BatchEqualsPerRequest(t *testing.T) {
	bldCred := builderWithdrawalCredentials()
	valCred := validatorWithdrawalCredentials()

	build := func() []*enginev1.DepositRequest {
		var reqs []*enginev1.DepositRequest
		for i := range 3 {
			sk, err := bls.RandKey()
			require.NoError(t, err)
			reqs = append(reqs, depositRequestFromPending(
				stateTesting.GeneratePendingDeposit(t, sk, uint64(100+i), bldCred, 0),
				uint64(len(reqs)),
			))
		}
		for i := range 2 {
			sk, err := bls.RandKey()
			require.NoError(t, err)
			reqs = append(reqs, depositRequestFromPending(
				stateTesting.GeneratePendingDeposit(t, sk, uint64(2000+i), valCred, 0),
				uint64(len(reqs)),
			))
		}
		return reqs
	}
	reqs := build()

	stBatch := newGloasState(t, nil, nil)
	require.NoError(t, processDepositRequests(t.Context(), stBatch, reqs, nil))

	stLegacy := newGloasState(t, nil, nil)
	require.NoError(t, processDepositRequestsPerRequest(stLegacy, reqs))

	rootBatch, err := stBatch.HashTreeRoot(t.Context())
	require.NoError(t, err)
	rootLegacy, err := stLegacy.HashTreeRoot(t.Context())
	require.NoError(t, err)
	require.DeepEqual(t, rootLegacy, rootBatch)
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
