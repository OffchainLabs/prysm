package gloas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestPreverifyBuilderDeposits(t *testing.T) {
	st, _ := util.DeterministicGenesisStateFulu(t, params.BeaconConfig().MaxValidatorsPerCommittee)

	builderOnlyKey, err := bls.RandKey()
	require.NoError(t, err)
	blockedBuilderKey, err := bls.RandKey()
	require.NoError(t, err)
	invalidBuilderKey, err := bls.RandKey()
	require.NoError(t, err)

	validatorCredentials := make([]byte, fieldparams.RootLength)
	pending := []*ethpb.PendingDeposit{
		newPendingDeposit(t, blockedBuilderKey, validatorCredentials, 5, 0, true),
		newPendingDeposit(t, blockedBuilderKey, builderWithdrawalCredentials(0xBB), 7, 0, true),
		newPendingDeposit(t, builderOnlyKey, builderWithdrawalCredentials(0xAA), 11, 0, true),
		newPendingDeposit(t, invalidBuilderKey, builderWithdrawalCredentials(0xCC), 13, 0, false),
	}
	require.NoError(t, st.SetPendingDeposits(pending))

	coldState, err := state_native.InitializeFromProtoFulu(st.ToProto().(*ethpb.BeaconStateFulu))
	require.NoError(t, err)
	result, err := gloas.PreverifyBuilderDeposits(t.Context(), st, len(pending))
	require.NoError(t, err)
	require.Equal(t, 2, result.ValidBuilderSignatures)
	require.Equal(t, 1, result.InvalidBuilderSignatures)
	require.Equal(t, 0, result.ValidValidatorSignatures)
	require.Equal(t, 0, result.InvalidValidatorSignatures)
	require.Equal(t, 3, result.CachedDeposits)
	require.Equal(t, 3, result.BuilderPubkeys)

	second, err := gloas.PreverifyBuilderDeposits(t.Context(), st, len(pending))
	require.NoError(t, err)
	require.Equal(t, 0, second.ValidBuilderSignatures)
	require.Equal(t, 0, second.InvalidBuilderSignatures)
	require.Equal(t, 1, second.ValidValidatorSignatures)
	require.Equal(t, len(pending), second.CachedDeposits)

	third, err := gloas.PreverifyBuilderDeposits(t.Context(), st, len(pending))
	require.NoError(t, err)
	require.Equal(t, 0, third.ValidBuilderSignatures)
	require.Equal(t, 0, third.InvalidBuilderSignatures)
	require.Equal(t, 0, third.ValidValidatorSignatures)
	require.Equal(t, len(pending), third.CachedDeposits)

	warm, err := gloas.UpgradeToGloas(st)
	require.NoError(t, err)
	cold, err := gloas.UpgradeToGloas(coldState)
	require.NoError(t, err)
	warmRoot, err := warm.HashTreeRoot(t.Context())
	require.NoError(t, err)
	coldRoot, err := cold.HashTreeRoot(t.Context())
	require.NoError(t, err)
	require.Equal(t, coldRoot, warmRoot)

	warmProto := warm.ToProtoUnsafe().(*ethpb.BeaconStateGloas)
	require.Equal(t, 1, len(warmProto.Builders))
	require.Equal(t, 2, len(warmProto.PendingDeposits))
}
