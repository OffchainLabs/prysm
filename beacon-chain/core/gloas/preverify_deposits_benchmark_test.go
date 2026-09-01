package gloas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func BenchmarkUpgradeToGloasBuilderDeposits(b *testing.B) {
	const depositCount = 4_096
	st, _ := util.DeterministicGenesisStateFulu(b, params.BeaconConfig().MaxValidatorsPerCommittee)
	pending := make([]*ethpb.PendingDeposit, depositCount)
	for i := range pending {
		sk, err := bls.RandKey()
		require.NoError(b, err)
		credentials := builderWithdrawalCredentials(byte(i))
		amount := uint64(32_000_000_000 + i)
		domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
		require.NoError(b, err)
		root, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
			PublicKey:             sk.PublicKey().Marshal(),
			WithdrawalCredentials: credentials,
			Amount:                amount,
		}, domain)
		require.NoError(b, err)
		pending[i] = &ethpb.PendingDeposit{
			PublicKey:             sk.PublicKey().Marshal(),
			WithdrawalCredentials: credentials,
			Amount:                amount,
			Signature:             sk.Sign(root[:]).Marshal(),
			Slot:                  primitives.Slot(i),
		}
	}
	require.NoError(b, st.SetPendingDeposits(pending))
	base := st.ToProto().(*ethpb.BeaconStateFulu)

	b.Run("preverify", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			state, err := state_native.InitializeFromProtoFulu(base)
			require.NoError(b, err)
			b.StartTimer()
			result, err := gloas.PreverifyBuilderDeposits(b.Context(), state, depositCount)
			require.NoError(b, err)
			require.Equal(b, depositCount, result.CachedDeposits)
		}
		b.ReportMetric(depositCount, "deposits/op")
	})

	for _, warm := range []bool{false, true} {
		name := "cold"
		if warm {
			name = "preverified"
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				state, err := state_native.InitializeFromProtoFulu(base)
				require.NoError(b, err)
				if warm {
					_, err = gloas.PreverifyBuilderDeposits(b.Context(), state, depositCount)
					require.NoError(b, err)
				}
				b.StartTimer()
				_, err = gloas.UpgradeToGloas(state)
				require.NoError(b, err)
			}
			b.ReportMetric(depositCount, "deposits/op")
		})
	}
}
