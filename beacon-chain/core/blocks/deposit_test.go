package blocks_test

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/container/trie"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
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
	verified, err := blocks.BatchVerifyDepositsSignatures(context.Background(), []*ethpb.Deposit{deposit})
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
	verified, err := blocks.BatchVerifyDepositsSignatures(context.Background(), []*ethpb.Deposit{deposit})
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
	err = blocks.VerifyDeposit(beaconState, deposit)
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
	valid, err := blocks.IsValidDepositSignature(depositData)
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

	verified, err := blocks.BatchVerifyPendingDepositsSignatures(context.Background(), []*ethpb.PendingDeposit{pendingDeposit, pendingDeposit2})
	require.NoError(t, err)
	require.Equal(t, true, verified)
}

func TestBatchVerifyPendingDepositsSignatures_InvalidSignature(t *testing.T) {
	pendingDeposit := &ethpb.PendingDeposit{
		PublicKey:             bytesutil.PadTo([]byte{1, 2, 3}, 48),
		WithdrawalCredentials: make([]byte, 32),
		Signature:             make([]byte, 96),
	}
	verified, err := blocks.BatchVerifyPendingDepositsSignatures(context.Background(), []*ethpb.PendingDeposit{pendingDeposit})
	require.NoError(t, err)
	require.Equal(t, false, verified)
}
