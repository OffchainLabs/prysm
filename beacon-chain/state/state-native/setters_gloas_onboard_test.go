package state_native

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// referenceOnboard is a deliberately naive transcription of onboard_builders_from_pending_deposits
// from specs/gloas/fork.md, used only as a differential oracle.
func referenceOnboard(pendingDeposits []*ethpb.PendingDeposit, validators []*ethpb.Validator) ([]*ethpb.PendingDeposit, []*ethpb.Builder, error) {
	validatorPubkeys := make([][]byte, 0, len(validators))
	for _, v := range validators {
		validatorPubkeys = append(validatorPubkeys, v.PublicKey)
	}
	hasPubkey := func(keys [][]byte, pubkey []byte) int {
		for i, k := range keys {
			if bytes.Equal(k, pubkey) {
				return i
			}
		}
		return -1
	}
	isValid := func(d *ethpb.PendingDeposit) (bool, error) {
		return helpers.IsValidDepositSignature(&ethpb.Deposit_Data{
			PublicKey:             d.PublicKey,
			WithdrawalCredentials: d.WithdrawalCredentials,
			Amount:                d.Amount,
			Signature:             d.Signature,
		})
	}

	kept := make([]*ethpb.PendingDeposit, 0, len(pendingDeposits))
	var builders []*ethpb.Builder
	for _, deposit := range pendingDeposits {
		if hasPubkey(validatorPubkeys, deposit.PublicKey) >= 0 {
			kept = append(kept, deposit)
			continue
		}
		builderPubkeys := make([][]byte, 0, len(builders))
		for _, b := range builders {
			builderPubkeys = append(builderPubkeys, b.Pubkey)
		}
		idx := hasPubkey(builderPubkeys, deposit.PublicKey)
		if idx < 0 {
			if !helpers.IsBuilderWithdrawalCredential(deposit.WithdrawalCredentials) {
				kept = append(kept, deposit)
				continue
			}
			pending := false
			for _, k := range kept {
				if !bytes.Equal(k.PublicKey, deposit.PublicKey) {
					continue
				}
				valid, err := isValid(k)
				if err != nil {
					return nil, nil, err
				}
				if valid {
					pending = true
					break
				}
			}
			if pending {
				kept = append(kept, deposit)
				continue
			}
			valid, err := isValid(deposit)
			if err != nil {
				return nil, nil, err
			}
			if !valid {
				continue
			}
			builders = append(builders, &ethpb.Builder{
				Pubkey:            deposit.PublicKey,
				Version:           []byte{params.BeaconConfig().PayloadBuilderVersion},
				ExecutionAddress:  deposit.WithdrawalCredentials[12:],
				Balance:           primitives.Gwei(deposit.Amount),
				DepositEpoch:      slots.ToEpoch(deposit.Slot),
				WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
			})
			continue
		}
		builders[idx].Balance += primitives.Gwei(deposit.Amount)
	}
	return kept, builders, nil
}

func requireMatchesReference(t *testing.T, validators []*ethpb.Validator, deposits []*ethpb.PendingDeposit) {
	t.Helper()
	wantKept, wantBuilders, err := referenceOnboard(deposits, validators)
	require.NoError(t, err)

	st := newGloasState(t, validators, nil, deposits, 0)
	require.NoError(t, st.OnboardBuildersFromPendingDeposits())

	require.Equal(t, len(wantKept), len(st.pendingDeposits))
	for i := range wantKept {
		require.DeepEqual(t, wantKept[i], st.pendingDeposits[i])
	}
	require.Equal(t, len(wantBuilders), len(st.builders))
	for i := range wantBuilders {
		require.DeepEqual(t, wantBuilders[i].Pubkey, st.builders[i].Pubkey)
		require.Equal(t, wantBuilders[i].Balance, st.builders[i].Balance)
		require.Equal(t, wantBuilders[i].DepositEpoch, st.builders[i].DepositEpoch)
		require.DeepEqual(t, wantBuilders[i].ExecutionAddress, st.builders[i].ExecutionAddress)
	}
}

// Equivalence oracle for the onboarding loop, standing in for the upstream fork spectest vectors.
func TestOnboardBuildersMatchesSpecReference(t *testing.T) {
	cache.DepositSignature.Clear()
	keys := make([]bls.SecretKey, 6)
	for i := range keys {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		keys[i] = sk
	}
	builderCreds := builderWithdrawalCredentials(0xA1)
	otherBuilderCreds := builderWithdrawalCredentials(0xB2)
	valCreds := nonBuilderWithdrawalCredentials()

	tests := []struct {
		name       string
		validators []*ethpb.Validator
		deposits   []*ethpb.PendingDeposit
	}{
		{
			name:     "empty queue",
			deposits: nil,
		},
		{
			name: "validator deposit then builder credentials same pubkey",
			deposits: []*ethpb.PendingDeposit{
				newPendingDeposit(t, keys[0], valCreds, 5, 0, true),
				newPendingDeposit(t, keys[0], builderCreds, 7, 0, true),
			},
		},
		{
			name: "invalid validator deposit then builder credentials same pubkey",
			deposits: []*ethpb.PendingDeposit{
				newPendingDeposit(t, keys[0], valCreds, 5, 0, false),
				newPendingDeposit(t, keys[0], builderCreds, 7, 0, true),
			},
		},
		{
			name: "invalid builder deposit then valid builder deposit",
			deposits: []*ethpb.PendingDeposit{
				newPendingDeposit(t, keys[1], builderCreds, 3, 0, false),
				newPendingDeposit(t, keys[1], builderCreds, 9, 0, true),
			},
		},
		{
			name: "valid builder deposit then invalid builder deposit tops up",
			deposits: []*ethpb.PendingDeposit{
				newPendingDeposit(t, keys[1], builderCreds, 9, 0, true),
				newPendingDeposit(t, keys[1], builderCreds, 3, 0, false),
			},
		},
		{
			name: "builder deposit then non builder credentials tops up",
			deposits: []*ethpb.PendingDeposit{
				newPendingDeposit(t, keys[2], builderCreds, 9, 0, true),
				newPendingDeposit(t, keys[2], valCreds, 3, 0, true),
			},
		},
		{
			name: "multiple deposits same builder",
			deposits: []*ethpb.PendingDeposit{
				newPendingDeposit(t, keys[2], builderCreds, 4, 0, true),
				newPendingDeposit(t, keys[2], builderCreds, 4, 0, true),
				newPendingDeposit(t, keys[2], builderCreds, 4, 0, true),
			},
		},
		{
			name: "multiple builders keep queue order",
			deposits: []*ethpb.PendingDeposit{
				newPendingDeposit(t, keys[3], builderCreds, 4, 0, true),
				newPendingDeposit(t, keys[4], otherBuilderCreds, 5, 0, true),
				newPendingDeposit(t, keys[5], builderCreds, 6, 0, true),
			},
		},
		{
			name:       "builder credentials for existing validator pubkey",
			validators: []*ethpb.Validator{{PublicKey: keys[0].PublicKey().Marshal()}},
			deposits: []*ethpb.PendingDeposit{
				newPendingDeposit(t, keys[0], builderCreds, 7, 0, false),
			},
		},
		{
			name: "mixed pending deposits",
			deposits: []*ethpb.PendingDeposit{
				newPendingDeposit(t, keys[0], valCreds, 1, 0, true),
				newPendingDeposit(t, keys[1], builderCreds, 2, 0, true),
				newPendingDeposit(t, keys[2], valCreds, 3, 0, false),
				newPendingDeposit(t, keys[3], builderCreds, 4, 0, false),
				newPendingDeposit(t, keys[4], otherBuilderCreds, 5, 0, true),
			},
		},
		{
			name: "builder deposit uses deposit slot epoch",
			deposits: []*ethpb.PendingDeposit{
				newPendingDeposit(t, keys[5], builderCreds, 8, primitives.Slot(params.BeaconConfig().SlotsPerEpoch*3+2), true),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireMatchesReference(t, tt.validators, tt.deposits)
		})
	}

	t.Run("randomized queues", func(t *testing.T) {
		rng := rand.New(rand.NewSource(1))
		credsFor := func(i int) []byte {
			switch i % 3 {
			case 0:
				return valCreds
			case 1:
				return builderCreds
			default:
				return otherBuilderCreds
			}
		}
		for round := range 40 {
			n := 1 + rng.Intn(10)
			deposits := make([]*ethpb.PendingDeposit, 0, n)
			for range n {
				sk := keys[rng.Intn(len(keys))]
				deposits = append(deposits, newPendingDeposit(
					t, sk, credsFor(rng.Intn(3)), uint64(1+rng.Intn(5)), primitives.Slot(rng.Intn(64)), rng.Intn(2) == 0,
				))
			}
			var validators []*ethpb.Validator
			if rng.Intn(4) == 0 {
				validators = []*ethpb.Validator{{PublicKey: keys[rng.Intn(len(keys))].PublicKey().Marshal()}}
			}
			t.Run(fmt.Sprintf("round %d", round), func(t *testing.T) {
				requireMatchesReference(t, validators, deposits)
			})
		}
	})
}

// The pre-fork warm-up must not change the fork's outcome.
func TestOnboardBuildersColdAndWarmCacheAgree(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	other, err := bls.RandKey()
	require.NoError(t, err)
	builderCreds := builderWithdrawalCredentials(0xC3)
	valCreds := nonBuilderWithdrawalCredentials()

	deposits := []*ethpb.PendingDeposit{
		newPendingDeposit(t, sk, valCreds, 5, 0, false),
		newPendingDeposit(t, sk, builderCreds, 7, 0, true),
		newPendingDeposit(t, other, builderCreds, 9, 0, false),
	}

	cache.DepositSignature.Clear()
	cold := newGloasState(t, nil, nil, ethpb.CopySlice(deposits), 0)
	require.NoError(t, cold.OnboardBuildersFromPendingDeposits())

	for _, d := range deposits {
		_, err := helpers.IsValidDepositSignature(&ethpb.Deposit_Data{
			PublicKey:             d.PublicKey,
			WithdrawalCredentials: d.WithdrawalCredentials,
			Amount:                d.Amount,
			Signature:             d.Signature,
		})
		require.NoError(t, err)
	}
	warm := newGloasState(t, nil, nil, ethpb.CopySlice(deposits), 0)
	require.NoError(t, warm.OnboardBuildersFromPendingDeposits())

	require.DeepEqual(t, cold.pendingDeposits, warm.pendingDeposits)
	require.DeepEqual(t, cold.builders, warm.builders)
	require.Equal(t, 1, len(warm.builders))
}

// Reduced-scale reproductions of the pre-fork scenarios in jtraglia/kurtosis-devnets/gloas-deposits.
func TestOnboardBuildersDevnetScenarios(t *testing.T) {
	builderCreds := builderWithdrawalCredentials(0xD4)
	valCreds := nonBuilderWithdrawalCredentials()

	t.Run("s2 valid builder deposits with unique pubkeys", func(t *testing.T) {
		cache.DepositSignature.Clear()
		const n = 64
		deposits := make([]*ethpb.PendingDeposit, 0, n)
		pubkeys := make([][]byte, 0, n)
		for range n {
			sk, err := bls.RandKey()
			require.NoError(t, err)
			pubkeys = append(pubkeys, sk.PublicKey().Marshal())
			deposits = append(deposits, newPendingDeposit(t, sk, builderCreds, 32, 0, true))
		}

		st := newGloasState(t, nil, nil, deposits, 0)
		require.NoError(t, st.OnboardBuildersFromPendingDeposits())
		require.Equal(t, 0, len(st.pendingDeposits))
		require.Equal(t, n, len(st.builders))
		for i, pubkey := range pubkeys {
			require.DeepEqual(t, pubkey, st.builders[i].Pubkey)
			idx, ok := st.builderIndexByPubkey(bytesutil48(pubkey))
			require.Equal(t, true, ok)
			require.Equal(t, primitives.BuilderIndex(i), idx)
		}
	})

	t.Run("s3 invalid builder deposits then one valid for same pubkey", func(t *testing.T) {
		cache.DepositSignature.Clear()
		const k = 128
		sk, err := bls.RandKey()
		require.NoError(t, err)
		deposits := make([]*ethpb.PendingDeposit, 0, k+1)
		for i := range k {
			d := newPendingDeposit(t, sk, builderCreds, 32, 0, false)
			d.Signature[0] = byte(i + 1)
			d.Signature[1] = byte(i >> 8)
			deposits = append(deposits, d)
		}
		deposits = append(deposits, newPendingDeposit(t, sk, builderCreds, 32, 0, true))

		st := newGloasState(t, nil, nil, deposits, 0)
		require.NoError(t, st.OnboardBuildersFromPendingDeposits())
		require.Equal(t, 0, len(st.pendingDeposits))
		require.Equal(t, 1, len(st.builders))
		require.DeepEqual(t, sk.PublicKey().Marshal(), st.builders[0].Pubkey)
		require.Equal(t, primitives.Gwei(32), st.builders[0].Balance)
	})

	t.Run("s5 invalid validator deposits then one valid builder deposit for same pubkey", func(t *testing.T) {
		cache.DepositSignature.Clear()
		const k = 128
		sk, err := bls.RandKey()
		require.NoError(t, err)
		deposits := make([]*ethpb.PendingDeposit, 0, k+1)
		for i := range k {
			d := newPendingDeposit(t, sk, valCreds, 32, 0, false)
			d.Signature[0] = byte(i + 1)
			d.Signature[1] = byte(i >> 8)
			deposits = append(deposits, d)
		}
		deposits = append(deposits, newPendingDeposit(t, sk, builderCreds, 32, 0, true))

		st := newGloasState(t, nil, nil, deposits, 0)
		require.NoError(t, st.OnboardBuildersFromPendingDeposits())
		require.Equal(t, k, len(st.pendingDeposits))
		require.Equal(t, 1, len(st.builders))
		require.Equal(t, primitives.Gwei(32), st.builders[0].Balance)
	})
}

// Outcome at the formerly quadratic shape. Linearity itself is pinned by
// TestPendingValidatorIndexDoesNotRescan.
func TestOnboardBuildersQuadraticShape(t *testing.T) {
	const k = 256
	builderCreds := builderWithdrawalCredentials(0xE5)
	valCreds := nonBuilderWithdrawalCredentials()
	sk, err := bls.RandKey()
	require.NoError(t, err)

	deposits := make([]*ethpb.PendingDeposit, 0, 2*k)
	for i := range k {
		d := newPendingDeposit(t, sk, valCreds, 32, 0, false)
		d.Signature[0] = byte(i + 1)
		d.Signature[1] = byte(i >> 8)
		deposits = append(deposits, d)
	}
	for i := range k {
		d := newPendingDeposit(t, sk, builderCreds, 32, 0, false)
		d.Signature[0] = byte(i + 1)
		d.Signature[1] = byte(i >> 8)
		d.Signature[2] = 0xff
		deposits = append(deposits, d)
	}

	cache.DepositSignature.Clear()
	st := newGloasState(t, nil, nil, deposits, 0)
	require.NoError(t, st.OnboardBuildersFromPendingDeposits())
	require.Equal(t, k, len(st.pendingDeposits))
	require.Equal(t, 0, len(st.builders))
	require.Equal(t, 2*k, cache.DepositSignature.Len())

	requireMatchesReference(t, nil, deposits)
}

func bytesutil48(b []byte) [fieldparams.BLSPubkeyLength]byte {
	var out [fieldparams.BLSPubkeyLength]byte
	copy(out[:], b)
	return out
}
