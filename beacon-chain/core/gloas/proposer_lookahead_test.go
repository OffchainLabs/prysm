package gloas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// Verifies the EIP-8045 filter: any validator marked slashed when the last
// lookahead epoch is computed must not appear in that epoch's proposer slots.
func TestProcessProposerLookahead_ExcludesSlashedValidators(t *testing.T) {
	ctx := t.Context()

	st, _ := util.DeterministicGenesisStateGloas(t, 256)

	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	lookaheadSize := (uint64(params.BeaconConfig().MinSeedLookahead) + 1) * slotsPerEpoch
	lastEpochStart := lookaheadSize - slotsPerEpoch

	// Baseline: take the unfiltered last-epoch lookahead so we can pick a
	// validator that is guaranteed to be selected, then verify the filtered
	// version drops it.
	baseline := st.Copy()
	require.NoError(t, gloas.ProcessProposerLookahead(ctx, baseline))
	baseLookahead, err := baseline.ProposerLookahead()
	require.NoError(t, err)

	target := baseLookahead[lastEpochStart]
	validators := st.Validators()
	validators[target].Slashed = true
	require.NoError(t, st.SetValidators(validators))

	require.NoError(t, gloas.ProcessProposerLookahead(ctx, st))
	filtered, err := st.ProposerLookahead()
	require.NoError(t, err)

	for i := lastEpochStart; i < lookaheadSize; i++ {
		require.NotEqual(t, target, filtered[i],
			"slashed validator %d still selected at lookahead slot %d", target, i)
	}

	// And confirm the filter actually changed something at the target slot.
	require.NotEqual(t, baseLookahead[lastEpochStart], filtered[lastEpochStart])
}
