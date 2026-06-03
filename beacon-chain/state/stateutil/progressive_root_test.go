package stateutil_test

import (
	"encoding/binary"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestValidatorRegistryRootProgressive(t *testing.T) {
	pubkey := make([]byte, fieldparams.BLSPubkeyLength)
	pubkey[0] = 1
	withdrawCreds := make([]byte, 32)
	withdrawCreds[0] = 2

	vals := []stateutil.CompactValidator{
		stateutil.CompactValidatorFromProto(&ethpb.Validator{}),
		stateutil.CompactValidatorFromProto(&ethpb.Validator{
			PublicKey:                  pubkey,
			WithdrawalCredentials:      withdrawCreds,
			EffectiveBalance:           32,
			Slashed:                    true,
			ActivationEligibilityEpoch: 1,
			ActivationEpoch:            2,
			ExitEpoch:                  3,
			WithdrawableEpoch:          4,
		}),
	}

	got, err := stateutil.ValidatorRegistryRootProgressive(vals)
	require.NoError(t, err)

	roots, err := stateutil.OptimizedValidatorRoots(vals)
	require.NoError(t, err)
	body := ssz.MerkleizeProgressiveChunks(roots)
	var length [32]byte
	binary.LittleEndian.PutUint64(length[:8], uint64(len(vals)))
	expected := ssz.MixInLength(body, length[:])
	require.Equal(t, expected, got)
}

func TestUint64ListRootProgressive(t *testing.T) {
	vals := []uint64{1, 2, 3, 4, 5, 6, 7}

	got, err := stateutil.Uint64ListRootProgressive(vals)
	require.NoError(t, err)

	chunks, err := stateutil.PackUint64IntoChunks(vals)
	require.NoError(t, err)
	body := ssz.MerkleizeProgressiveChunks(chunks)
	var length [32]byte
	binary.LittleEndian.PutUint64(length[:8], uint64(len(vals)))
	expected := ssz.MixInLength(body, length[:])
	require.Equal(t, expected, got)
}

func TestParticipationBitsRootProgressive(t *testing.T) {
	bits := []byte{0x01, 0x02, 0x03, 0x04}

	got, err := stateutil.ParticipationBitsRootProgressive(bits)
	require.NoError(t, err)

	expected, err := ssz.ByteSliceRootProgressive(bits)
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func TestPendingRootsProgressive(t *testing.T) {
	pendingDeposits := []*ethpb.PendingDeposit{{
		PublicKey:             make([]byte, fieldparams.BLSPubkeyLength),
		WithdrawalCredentials: make([]byte, 32),
		Signature:             make([]byte, fieldparams.BLSSignatureLength),
	}}
	pdRoot, err := stateutil.PendingDepositsRootProgressive(pendingDeposits)
	require.NoError(t, err)
	expectedPD, err := ssz.SliceRootProgressive(pendingDeposits)
	require.NoError(t, err)
	require.Equal(t, expectedPD, pdRoot)

	pendingPartialWithdrawals := []*ethpb.PendingPartialWithdrawal{{}}
	ppwRoot, err := stateutil.PendingPartialWithdrawalsRootProgressive(pendingPartialWithdrawals)
	require.NoError(t, err)
	expectedPPW, err := ssz.SliceRootProgressive(pendingPartialWithdrawals)
	require.NoError(t, err)
	require.Equal(t, expectedPPW, ppwRoot)

	pendingConsolidations := []*ethpb.PendingConsolidation{{}}
	pcRoot, err := stateutil.PendingConsolidationsRootProgressive(pendingConsolidations)
	require.NoError(t, err)
	expectedPC, err := ssz.SliceRootProgressive(pendingConsolidations)
	require.NoError(t, err)
	require.Equal(t, expectedPC, pcRoot)

	builderPendingWithdrawals := []*ethpb.BuilderPendingWithdrawal{{
		FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
		Amount:       1,
		BuilderIndex: 2,
	}}
	bpwRoot, err := stateutil.BuilderPendingWithdrawalsRootProgressive(builderPendingWithdrawals)
	require.NoError(t, err)
	expectedBPW, err := ssz.SliceRootProgressive(builderPendingWithdrawals)
	require.NoError(t, err)
	require.Equal(t, expectedBPW, bpwRoot)
}
