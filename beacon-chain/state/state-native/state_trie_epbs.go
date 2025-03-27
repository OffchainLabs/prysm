package state_native

import (
	"runtime"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/fieldtrie"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
)

// InitializeFromProtoUnsafeEpbs constructs a BeaconState from its protobuf representation.
func InitializeFromProtoUnsafeEpbs(st *ethpb.BeaconStateEPBS) (*BeaconState, error) {
	if st == nil {
		return nil, errors.New("received nil state")
	}

	// Process historical roots.
	hRoots := make([][32]byte, len(st.HistoricalRoots))
	for i, root := range st.HistoricalRoots {
		hRoots[i] = bytesutil.ToBytes32(root)
	}

	// Define the number of fields to track changes.
	fieldCount := params.BeaconConfig().BeaconStateEpbsFieldCount
	b := &BeaconState{
		version:                       version.EPBS,
		genesisTime:                   st.GenesisTime,
		genesisValidatorsRoot:         bytesutil.ToBytes32(st.GenesisValidatorsRoot),
		slot:                          st.Slot,
		fork:                          st.Fork,
		latestBlockHeader:             st.LatestBlockHeader,
		historicalRoots:               hRoots,
		eth1Data:                      st.Eth1Data,
		eth1DataVotes:                 st.Eth1DataVotes,
		eth1DepositIndex:              st.Eth1DepositIndex,
		slashings:                     st.Slashings,
		previousEpochParticipation:    st.PreviousEpochParticipation,
		currentEpochParticipation:     st.CurrentEpochParticipation,
		justificationBits:             st.JustificationBits,
		previousJustifiedCheckpoint:   st.PreviousJustifiedCheckpoint,
		currentJustifiedCheckpoint:    st.CurrentJustifiedCheckpoint,
		finalizedCheckpoint:           st.FinalizedCheckpoint,
		currentSyncCommittee:          st.CurrentSyncCommittee,
		nextSyncCommittee:             st.NextSyncCommittee,
		nextWithdrawalIndex:           st.NextWithdrawalIndex,
		nextWithdrawalValidatorIndex:  st.NextWithdrawalValidatorIndex,
		historicalSummaries:           st.HistoricalSummaries,
		depositRequestsStartIndex:     st.DepositRequestsStartIndex,
		depositBalanceToConsume:       st.DepositBalanceToConsume,
		exitBalanceToConsume:          st.ExitBalanceToConsume,
		earliestExitEpoch:             st.EarliestExitEpoch,
		consolidationBalanceToConsume: st.ConsolidationBalanceToConsume,
		earliestConsolidationEpoch:    st.EarliestConsolidationEpoch,
		pendingDeposits:               st.PendingDeposits,
		pendingPartialWithdrawals:     st.PendingPartialWithdrawals,
		pendingConsolidations:         st.PendingConsolidations,

		// ePBS fields
		latestBlockHash:                  bytesutil.ToBytes32(st.LatestBlockHash),
		latestFullSlot:                   st.LatestFullSlot,
		latestExecutionPayloadHeaderEPBS: st.LatestExecutionPayloadHeader,
		latestWithdrawalsRoot:            bytesutil.ToBytes32(st.LatestWithdrawalsRoot),

		dirtyFields:      make(map[types.FieldIndex]bool, fieldCount),
		dirtyIndices:     make(map[types.FieldIndex][]uint64, fieldCount),
		stateFieldLeaves: make(map[types.FieldIndex]*fieldtrie.FieldTrie, fieldCount),
		rebuildTrie:      make(map[types.FieldIndex]bool, fieldCount),
		valMapHandler:    stateutil.NewValMapHandler(st.Validators),
	}

	b.blockRootsMultiValue = NewMultiValueBlockRoots(st.BlockRoots)
	b.stateRootsMultiValue = NewMultiValueStateRoots(st.StateRoots)
	b.randaoMixesMultiValue = NewMultiValueRandaoMixes(st.RandaoMixes)
	b.balancesMultiValue = NewMultiValueBalances(st.Balances)
	b.validatorsMultiValue = NewMultiValueValidators(st.Validators)
	b.inactivityScoresMultiValue = NewMultiValueInactivityScores(st.InactivityScores)
	b.sharedFieldReferences = make(map[types.FieldIndex]*stateutil.Reference, epbsSharedFieldRefCount)

	for _, f := range epbsFields {
		b.dirtyFields[f] = true
		b.rebuildTrie[f] = true
		b.dirtyIndices[f] = []uint64{}
		trie, err := fieldtrie.NewFieldTrie(f, types.BasicArray, nil, 0)
		if err != nil {
			return nil, err
		}
		b.stateFieldLeaves[f] = trie
	}

	// Initialize field reference tracking for shared data.
	b.sharedFieldReferences[types.HistoricalRoots] = stateutil.NewRef(1)
	b.sharedFieldReferences[types.Eth1DataVotes] = stateutil.NewRef(1)
	b.sharedFieldReferences[types.Slashings] = stateutil.NewRef(1)
	b.sharedFieldReferences[types.PreviousEpochParticipationBits] = stateutil.NewRef(1)
	b.sharedFieldReferences[types.CurrentEpochParticipationBits] = stateutil.NewRef(1)
	b.sharedFieldReferences[types.HistoricalSummaries] = stateutil.NewRef(1)
	b.sharedFieldReferences[types.PendingDeposits] = stateutil.NewRef(1)
	b.sharedFieldReferences[types.PendingPartialWithdrawals] = stateutil.NewRef(1)
	b.sharedFieldReferences[types.PendingConsolidations] = stateutil.NewRef(1)

	state.Count.Inc()
	// Finalizer runs when dst is being destroyed in garbage collection.
	runtime.SetFinalizer(b, finalizerCleanup)
	return b, nil
}
