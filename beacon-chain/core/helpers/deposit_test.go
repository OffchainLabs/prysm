package helpers_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBatchVerifyDepositsSignatures_Ok(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	deposit := &ethpb.Deposit{
		Data: &ethpb.Deposit_Data{
			PublicKey:             sk.PublicKey().Marshal(),
			WithdrawalCredentials: make([]byte, 32),
			Amount:                3000,
		},
	}
	sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
		PublicKey:             deposit.Data.PublicKey,
		WithdrawalCredentials: deposit.Data.WithdrawalCredentials,
		Amount:                3000,
	}, domain)
	require.NoError(t, err)
	sig := sk.Sign(sr[:])
	deposit.Data.Signature = sig.Marshal()
	leaf, err := deposit.Data.HashTreeRoot()
	require.NoError(t, err)
	// We then create a merkle branch for the test.
	depositTrie, err := trie.GenerateTrieFromItems([][]byte{leaf[:]}, params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not generate trie")
	proof, err := depositTrie.MerkleProof(0)
	require.NoError(t, err, "Could not generate proof")

	deposit.Proof = proof
	require.NoError(t, err)
	verified, err := helpers.BatchVerifyDepositsSignatures(t.Context(), []*ethpb.Deposit{deposit})
	require.NoError(t, err)
	require.Equal(t, true, verified)
}

func TestBatchVerifyDepositsSignatures_InvalidSignature(t *testing.T) {
	deposit := &ethpb.Deposit{
		Data: &ethpb.Deposit_Data{
			PublicKey:             bytesutil.PadTo([]byte{1, 2, 3}, 48),
			WithdrawalCredentials: make([]byte, 32),
			Signature:             make([]byte, 96),
		},
	}
	leaf, err := deposit.Data.HashTreeRoot()
	require.NoError(t, err)
	// We then create a merkle branch for the test.
	depositTrie, err := trie.GenerateTrieFromItems([][]byte{leaf[:]}, params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not generate trie")
	proof, err := depositTrie.MerkleProof(0)
	require.NoError(t, err, "Could not generate proof")

	deposit.Proof = proof
	require.NoError(t, err)
	verified, err := helpers.BatchVerifyDepositsSignatures(t.Context(), []*ethpb.Deposit{deposit})
	require.NoError(t, err)
	require.Equal(t, false, verified)
}

func TestVerifyDeposit_MerkleBranchFailsVerification(t *testing.T) {
	deposit := &ethpb.Deposit{
		Data: &ethpb.Deposit_Data{
			PublicKey:             bytesutil.PadTo([]byte{1, 2, 3}, 48),
			WithdrawalCredentials: make([]byte, 32),
			Signature:             make([]byte, 96),
		},
	}
	leaf, err := deposit.Data.HashTreeRoot()
	require.NoError(t, err)

	// We then create a merkle branch for the test.
	depositTrie, err := trie.GenerateTrieFromItems([][]byte{leaf[:]}, params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not generate trie")
	proof, err := depositTrie.MerkleProof(0)
	require.NoError(t, err, "Could not generate proof")

	deposit.Proof = proof
	beaconState, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: []byte{0},
			BlockHash:   []byte{1},
		},
	})
	require.NoError(t, err)
	want := "deposit root did not verify"
	err = helpers.VerifyDeposit(beaconState, deposit)
	require.ErrorContains(t, want, err)
}

func TestIsValidDepositSignature_Ok(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	depositData := &ethpb.Deposit_Data{
		PublicKey:             sk.PublicKey().Marshal(),
		Amount:                0,
		WithdrawalCredentials: make([]byte, 32),
		Signature:             make([]byte, fieldparams.BLSSignatureLength),
	}
	dm := &ethpb.DepositMessage{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: make([]byte, 32),
		Amount:                0,
	}
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	sr, err := signing.ComputeSigningRoot(dm, domain)
	require.NoError(t, err)
	sig := sk.Sign(sr[:])
	depositData.Signature = sig.Marshal()
	valid, err := helpers.IsValidDepositSignature(depositData)
	require.NoError(t, err)
	require.Equal(t, true, valid)
}

func TestBatchVerifyPendingDepositsSignatures_Ok(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	pendingDeposit := &ethpb.PendingDeposit{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: make([]byte, 32),
		Amount:                3000,
	}
	sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
		PublicKey:             pendingDeposit.PublicKey,
		WithdrawalCredentials: pendingDeposit.WithdrawalCredentials,
		Amount:                3000,
	}, domain)
	require.NoError(t, err)
	sig := sk.Sign(sr[:])
	pendingDeposit.Signature = sig.Marshal()

	sk2, err := bls.RandKey()
	require.NoError(t, err)
	pendingDeposit2 := &ethpb.PendingDeposit{
		PublicKey:             sk2.PublicKey().Marshal(),
		WithdrawalCredentials: make([]byte, 32),
		Amount:                4000,
	}
	sr2, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
		PublicKey:             pendingDeposit2.PublicKey,
		WithdrawalCredentials: pendingDeposit2.WithdrawalCredentials,
		Amount:                4000,
	}, domain)
	require.NoError(t, err)
	sig2 := sk2.Sign(sr2[:])
	pendingDeposit2.Signature = sig2.Marshal()

	verified, err := helpers.BatchVerifyPendingDepositsSignatures(t.Context(), []*ethpb.PendingDeposit{pendingDeposit, pendingDeposit2})
	require.NoError(t, err)
	require.Equal(t, true, verified)
}

func TestBatchVerifyPendingDepositsSignatures_InvalidSignature(t *testing.T) {
	pendingDeposit := &ethpb.PendingDeposit{
		PublicKey:             bytesutil.PadTo([]byte{1, 2, 3}, 48),
		WithdrawalCredentials: make([]byte, 32),
		Signature:             make([]byte, 96),
	}
	verified, err := helpers.BatchVerifyPendingDepositsSignatures(t.Context(), []*ethpb.PendingDeposit{pendingDeposit})
	require.NoError(t, err)
	require.Equal(t, false, verified)
}

func makeValidDepositRequest(t *testing.T, amount uint64) *enginev1.DepositRequest {
	t.Helper()
	sk, err := bls.RandKey()
	require.NoError(t, err)
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	wc := make([]byte, 32)
	sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: wc,
		Amount:                amount,
	}, domain)
	require.NoError(t, err)
	return &enginev1.DepositRequest{
		Pubkey:                sk.PublicKey().Marshal(),
		WithdrawalCredentials: wc,
		Amount:                amount,
		Signature:             sk.Sign(sr[:]).Marshal(),
	}
}

func makeInvalidDepositRequest(t *testing.T, amount uint64) *enginev1.DepositRequest {
	t.Helper()
	sk, err := bls.RandKey()
	require.NoError(t, err)
	return &enginev1.DepositRequest{
		Pubkey:                sk.PublicKey().Marshal(),
		WithdrawalCredentials: make([]byte, 32),
		Amount:                amount,
		Signature:             make([]byte, 96),
	}
}

func TestBatchVerifyDepositRequestSignatures_Empty(t *testing.T) {
	invalid, err := helpers.BatchVerifyDepositRequestSignatures(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, 0, len(invalid))
}

func TestBatchVerifyDepositRequestSignatures_AllValid(t *testing.T) {
	reqs := []*enginev1.DepositRequest{
		makeValidDepositRequest(t, 100),
		makeValidDepositRequest(t, 200),
		makeValidDepositRequest(t, 300),
		makeValidDepositRequest(t, 400),
	}
	invalid, err := helpers.BatchVerifyDepositRequestSignatures(t.Context(), reqs)
	require.NoError(t, err)
	require.Equal(t, 0, len(invalid))
}

func TestBatchVerifyDepositRequestSignatures_AllInvalid(t *testing.T) {
	reqs := []*enginev1.DepositRequest{
		makeInvalidDepositRequest(t, 100),
		makeInvalidDepositRequest(t, 200),
		makeInvalidDepositRequest(t, 300),
	}
	invalid, err := helpers.BatchVerifyDepositRequestSignatures(t.Context(), reqs)
	require.NoError(t, err)
	require.DeepEqual(t, []int{0, 1, 2}, invalid)
}

func TestBatchVerifyDepositRequestSignatures_SingleValid(t *testing.T) {
	invalid, err := helpers.BatchVerifyDepositRequestSignatures(t.Context(), []*enginev1.DepositRequest{makeValidDepositRequest(t, 1)})
	require.NoError(t, err)
	require.Equal(t, 0, len(invalid))
}

func TestBatchVerifyDepositRequestSignatures_SingleInvalid(t *testing.T) {
	invalid, err := helpers.BatchVerifyDepositRequestSignatures(t.Context(), []*enginev1.DepositRequest{makeInvalidDepositRequest(t, 1)})
	require.NoError(t, err)
	require.DeepEqual(t, []int{0}, invalid)
}

func TestBatchVerifyDepositRequestSignatures_MixedDC(t *testing.T) {
	reqs := []*enginev1.DepositRequest{
		makeInvalidDepositRequest(t, 1),
		makeValidDepositRequest(t, 2),
		makeValidDepositRequest(t, 3),
		makeInvalidDepositRequest(t, 4),
		makeValidDepositRequest(t, 5),
		makeValidDepositRequest(t, 6),
		makeInvalidDepositRequest(t, 7),
		makeValidDepositRequest(t, 8),
	}
	invalid, err := helpers.BatchVerifyDepositRequestSignatures(t.Context(), reqs)
	require.NoError(t, err)
	require.DeepEqual(t, []int{0, 3, 6}, invalid)
}

func TestBatchVerifyDepositRequestSignatures_OneBadInLargeBatch(t *testing.T) {
	const n = 128
	reqs := make([]*enginev1.DepositRequest, n)
	for i := range n {
		reqs[i] = makeValidDepositRequest(t, uint64(i+1))
	}
	const badIdx = 11
	reqs[badIdx] = makeInvalidDepositRequest(t, badIdx+1)
	invalid, err := helpers.BatchVerifyDepositRequestSignatures(t.Context(), reqs)
	require.NoError(t, err)
	require.DeepEqual(t, []int{badIdx}, invalid)
}

func TestBatchVerifyDepositRequestSignatures_MultipleBadAcrossSubtrees(t *testing.T) {
	const n = 128
	reqs := make([]*enginev1.DepositRequest, n)
	for i := range n {
		reqs[i] = makeValidDepositRequest(t, uint64(i+1))
	}
	badIdxs := []int{5, 47, 99, 120}
	for _, idx := range badIdxs {
		reqs[idx] = makeInvalidDepositRequest(t, uint64(idx+1))
	}
	invalid, err := helpers.BatchVerifyDepositRequestSignatures(t.Context(), reqs)
	require.NoError(t, err)
	require.DeepEqual(t, badIdxs, invalid)
}
