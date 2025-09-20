package state_native

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// initializeFieldRoots initializes the field roots slice based on the beacon state version.
func initializeFieldRoots(state *BeaconState) ([][]byte, error) {
	var fieldRoots [][]byte
	switch state.version {
	case version.Phase0:
		fieldRoots = make([][]byte, params.BeaconConfig().BeaconStateFieldCount)
	case version.Altair:
		fieldRoots = make([][]byte, params.BeaconConfig().BeaconStateAltairFieldCount)
	case version.Bellatrix:
		fieldRoots = make([][]byte, params.BeaconConfig().BeaconStateBellatrixFieldCount)
	case version.Capella:
		fieldRoots = make([][]byte, params.BeaconConfig().BeaconStateCapellaFieldCount)
	case version.Deneb:
		fieldRoots = make([][]byte, params.BeaconConfig().BeaconStateDenebFieldCount)
	case version.Electra:
		fieldRoots = make([][]byte, params.BeaconConfig().BeaconStateElectraFieldCount)
	case version.Fulu:
		fieldRoots = make([][]byte, params.BeaconConfig().BeaconStateFuluFieldCount)
	default:
		return nil, fmt.Errorf("unknown state version %s", version.String(state.version))
	}
	return fieldRoots, nil
}

// computeBasicFieldRoots computes basic field roots that are common across all versions.
func computeBasicFieldRoots(state *BeaconState, fieldRoots [][]byte) error {
	// Genesis time root.
	genesisRoot := ssz.Uint64Root(state.genesisTime)
	fieldRoots[types.GenesisTime.RealPosition()] = genesisRoot[:]

	// Genesis validators root.
	var r [32]byte
	copy(r[:], state.genesisValidatorsRoot[:])
	fieldRoots[types.GenesisValidatorsRoot.RealPosition()] = r[:]

	// Slot root.
	slotRoot := ssz.Uint64Root(uint64(state.slot))
	fieldRoots[types.Slot.RealPosition()] = slotRoot[:]

	// Fork data structure root.
	forkHashTreeRoot, err := ssz.ForkRoot(state.fork)
	if err != nil {
		return errors.Wrap(err, "could not compute fork merkleization")
	}
	fieldRoots[types.Fork.RealPosition()] = forkHashTreeRoot[:]

	// BeaconBlockHeader data structure root.
	headerHashTreeRoot, err := stateutil.BlockHeaderRoot(state.latestBlockHeader)
	if err != nil {
		return errors.Wrap(err, "could not compute block header merkleization")
	}
	fieldRoots[types.LatestBlockHeader.RealPosition()] = headerHashTreeRoot[:]

	// Eth1Data data structure root.
	eth1HashTreeRoot, err := stateutil.Eth1Root(state.eth1Data)
	if err != nil {
		return errors.Wrap(err, "could not compute eth1data merkleization")
	}
	fieldRoots[types.Eth1Data.RealPosition()] = eth1HashTreeRoot[:]

	// Eth1DepositIndex root.
	eth1DepositIndexBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(eth1DepositIndexBuf, state.eth1DepositIndex)
	eth1DepositBuf := bytesutil.ToBytes32(eth1DepositIndexBuf)
	fieldRoots[types.Eth1DepositIndex.RealPosition()] = eth1DepositBuf[:]

	return nil
}

// computeArrayFieldRoots computes field roots for arrays and slices that are common across all versions.
func computeArrayFieldRoots(state *BeaconState, fieldRoots [][]byte) error {
	// BlockRoots array root.
	blockRootsRoot, err := stateutil.ArraysRoot(state.blockRootsVal().Slice(), fieldparams.BlockRootsLength)
	if err != nil {
		return errors.Wrap(err, "could not compute block roots merkleization")
	}
	fieldRoots[types.BlockRoots.RealPosition()] = blockRootsRoot[:]

	// StateRoots array root.
	stateRootsRoot, err := stateutil.ArraysRoot(state.stateRootsVal().Slice(), fieldparams.StateRootsLength)
	if err != nil {
		return errors.Wrap(err, "could not compute state roots merkleization")
	}
	fieldRoots[types.StateRoots.RealPosition()] = stateRootsRoot[:]

	// HistoricalRoots slice root.
	hRoots := make([][]byte, len(state.historicalRoots))
	for i := range hRoots {
		hRoots[i] = state.historicalRoots[i][:]
	}
	historicalRootsRt, err := ssz.ByteArrayRootWithLimit(hRoots, fieldparams.HistoricalRootsLength)
	if err != nil {
		return errors.Wrap(err, "could not compute historical roots merkleization")
	}
	fieldRoots[types.HistoricalRoots.RealPosition()] = historicalRootsRt[:]

	// Eth1DataVotes slice root.
	eth1VotesRoot, err := stateutil.Eth1DataVotesRoot(state.eth1DataVotes)
	if err != nil {
		return errors.Wrap(err, "could not compute eth1data votes merkleization")
	}
	fieldRoots[types.Eth1DataVotes.RealPosition()] = eth1VotesRoot[:]

	// Validators slice root.
	validatorsRoot, err := stateutil.ValidatorRegistryRoot(state.validatorsVal())
	if err != nil {
		return errors.Wrap(err, "could not compute validator registry merkleization")
	}
	fieldRoots[types.Validators.RealPosition()] = validatorsRoot[:]

	// Balances slice root.
	balancesRoot, err := stateutil.Uint64ListRootWithRegistryLimit(state.balancesVal())
	if err != nil {
		return errors.Wrap(err, "could not compute validator balances merkleization")
	}
	fieldRoots[types.Balances.RealPosition()] = balancesRoot[:]

	// RandaoMixes array root.
	randaoRootsRoot, err := stateutil.ArraysRoot(state.randaoMixesVal().Slice(), fieldparams.RandaoMixesLength)
	if err != nil {
		return errors.Wrap(err, "could not compute randao roots merkleization")
	}
	fieldRoots[types.RandaoMixes.RealPosition()] = randaoRootsRoot[:]

	// Slashings array root.
	slashingsRootsRoot, err := ssz.SlashingsRoot(state.slashings)
	if err != nil {
		return errors.Wrap(err, "could not compute slashings merkleization")
	}
	fieldRoots[types.Slashings.RealPosition()] = slashingsRootsRoot[:]

	return nil
}

// computeCheckpointFieldRoots computes field roots for checkpoint-related fields.
func computeCheckpointFieldRoots(state *BeaconState, fieldRoots [][]byte) error {
	// JustificationBits root.
	justifiedBitsRoot := bytesutil.ToBytes32(state.justificationBits)
	fieldRoots[types.JustificationBits.RealPosition()] = justifiedBitsRoot[:]

	// PreviousJustifiedCheckpoint data structure root.
	prevCheckRoot, err := ssz.CheckpointRoot(state.previousJustifiedCheckpoint)
	if err != nil {
		return errors.Wrap(err, "could not compute previous justified checkpoint merkleization")
	}
	fieldRoots[types.PreviousJustifiedCheckpoint.RealPosition()] = prevCheckRoot[:]

	// CurrentJustifiedCheckpoint data structure root.
	currJustRoot, err := ssz.CheckpointRoot(state.currentJustifiedCheckpoint)
	if err != nil {
		return errors.Wrap(err, "could not compute current justified checkpoint merkleization")
	}
	fieldRoots[types.CurrentJustifiedCheckpoint.RealPosition()] = currJustRoot[:]

	// FinalizedCheckpoint data structure root.
	finalRoot, err := ssz.CheckpointRoot(state.finalizedCheckpoint)
	if err != nil {
		return errors.Wrap(err, "could not compute finalized checkpoint merkleization")
	}
	fieldRoots[types.FinalizedCheckpoint.RealPosition()] = finalRoot[:]

	return nil
}

// computePhase0SpecificRoots computes field roots specific to Phase 0.
func computePhase0SpecificRoots(state *BeaconState, fieldRoots [][]byte) error {
	if state.version != version.Phase0 {
		return nil
	}

	// PreviousEpochAttestations slice root.
	prevAttsRoot, err := stateutil.EpochAttestationsRoot(state.previousEpochAttestations)
	if err != nil {
		return errors.Wrap(err, "could not compute previous epoch attestations merkleization")
	}
	fieldRoots[types.PreviousEpochAttestations.RealPosition()] = prevAttsRoot[:]

	// CurrentEpochAttestations slice root.
	currAttsRoot, err := stateutil.EpochAttestationsRoot(state.currentEpochAttestations)
	if err != nil {
		return errors.Wrap(err, "could not compute current epoch attestations merkleization")
	}
	fieldRoots[types.CurrentEpochAttestations.RealPosition()] = currAttsRoot[:]

	return nil
}

// computeAltairPlusRoots computes field roots for Altair and later versions.
func computeAltairPlusRoots(state *BeaconState, fieldRoots [][]byte) error {
	if state.version < version.Altair {
		return nil
	}

	// PreviousEpochParticipation slice root.
	prevParticipationRoot, err := stateutil.ParticipationBitsRoot(state.previousEpochParticipation)
	if err != nil {
		return errors.Wrap(err, "could not compute previous epoch participation merkleization")
	}
	fieldRoots[types.PreviousEpochParticipationBits.RealPosition()] = prevParticipationRoot[:]

	// CurrentEpochParticipation slice root.
	currParticipationRoot, err := stateutil.ParticipationBitsRoot(state.currentEpochParticipation)
	if err != nil {
		return errors.Wrap(err, "could not compute current epoch participation merkleization")
	}
	fieldRoots[types.CurrentEpochParticipationBits.RealPosition()] = currParticipationRoot[:]

	// Inactivity scores root.
	inactivityScoresRoot, err := stateutil.Uint64ListRootWithRegistryLimit(state.inactivityScoresVal())
	if err != nil {
		return errors.Wrap(err, "could not compute inactivityScoreRoot")
	}
	fieldRoots[types.InactivityScores.RealPosition()] = inactivityScoresRoot[:]

	// Current sync committee root.
	currentSyncCommitteeRoot, err := stateutil.SyncCommitteeRoot(state.currentSyncCommittee)
	if err != nil {
		return errors.Wrap(err, "could not compute sync committee merkleization")
	}
	fieldRoots[types.CurrentSyncCommittee.RealPosition()] = currentSyncCommitteeRoot[:]

	// Next sync committee root.
	nextSyncCommitteeRoot, err := stateutil.SyncCommitteeRoot(state.nextSyncCommittee)
	if err != nil {
		return errors.Wrap(err, "could not compute sync committee merkleization")
	}
	fieldRoots[types.NextSyncCommittee.RealPosition()] = nextSyncCommitteeRoot[:]

	return nil
}

// computeExecutionPayloadRoots computes execution payload roots for different versions.
func computeExecutionPayloadRoots(state *BeaconState, fieldRoots [][]byte) error {
	switch state.version {
	case version.Bellatrix:
		// Execution payload root.
		executionPayloadRoot, err := state.latestExecutionPayloadHeader.HashTreeRoot()
		if err != nil {
			return err
		}
		fieldRoots[types.LatestExecutionPayloadHeader.RealPosition()] = executionPayloadRoot[:]

	case version.Capella:
		// Execution payload root.
		executionPayloadRoot, err := state.latestExecutionPayloadHeaderCapella.HashTreeRoot()
		if err != nil {
			return err
		}
		fieldRoots[types.LatestExecutionPayloadHeaderCapella.RealPosition()] = executionPayloadRoot[:]

	default:
		if state.version >= version.Deneb {
			// Execution payload root.
			executionPayloadRoot, err := state.latestExecutionPayloadHeaderDeneb.HashTreeRoot()
			if err != nil {
				return err
			}
			fieldRoots[types.LatestExecutionPayloadHeaderDeneb.RealPosition()] = executionPayloadRoot[:]
		}
	}

	return nil
}

// computeCapellaRoots computes field roots for Capella and later versions.
func computeCapellaRoots(state *BeaconState, fieldRoots [][]byte) error {
	if state.version < version.Capella {
		return nil
	}

	// Next withdrawal index root.
	nextWithdrawalIndexRoot := make([]byte, 32)
	binary.LittleEndian.PutUint64(nextWithdrawalIndexRoot, state.nextWithdrawalIndex)
	fieldRoots[types.NextWithdrawalIndex.RealPosition()] = nextWithdrawalIndexRoot

	// Next partial withdrawal validator index root.
	nextWithdrawalValidatorIndexRoot := make([]byte, 32)
	binary.LittleEndian.PutUint64(nextWithdrawalValidatorIndexRoot, uint64(state.nextWithdrawalValidatorIndex))
	fieldRoots[types.NextWithdrawalValidatorIndex.RealPosition()] = nextWithdrawalValidatorIndexRoot

	// Historical summary root.
	historicalSummaryRoot, err := stateutil.HistoricalSummariesRoot(state.historicalSummaries)
	if err != nil {
		return errors.Wrap(err, "could not compute historical summary merkleization")
	}
	fieldRoots[types.HistoricalSummaries.RealPosition()] = historicalSummaryRoot[:]

	return nil
}

// computeElectraRoots computes field roots for Electra version.
func computeElectraRoots(state *BeaconState, fieldRoots [][]byte) error {
	if state.version < version.Electra {
		return nil
	}

	// DepositRequestsStartIndex root.
	drsiRoot := ssz.Uint64Root(state.depositRequestsStartIndex)
	fieldRoots[types.DepositRequestsStartIndex.RealPosition()] = drsiRoot[:]

	// DepositBalanceToConsume root.
	dbtcRoot := ssz.Uint64Root(uint64(state.depositBalanceToConsume))
	fieldRoots[types.DepositBalanceToConsume.RealPosition()] = dbtcRoot[:]

	// ExitBalanceToConsume root.
	ebtcRoot := ssz.Uint64Root(uint64(state.exitBalanceToConsume))
	fieldRoots[types.ExitBalanceToConsume.RealPosition()] = ebtcRoot[:]

	// EarliestExitEpoch root.
	eeeRoot := ssz.Uint64Root(uint64(state.earliestExitEpoch))
	fieldRoots[types.EarliestExitEpoch.RealPosition()] = eeeRoot[:]

	// ConsolidationBalanceToConsume root.
	cbtcRoot := ssz.Uint64Root(uint64(state.consolidationBalanceToConsume))
	fieldRoots[types.ConsolidationBalanceToConsume.RealPosition()] = cbtcRoot[:]

	// EarliestConsolidationEpoch root.
	eceRoot := ssz.Uint64Root(uint64(state.earliestConsolidationEpoch))
	fieldRoots[types.EarliestConsolidationEpoch.RealPosition()] = eceRoot[:]

	// PendingDeposits root.
	pbdRoot, err := stateutil.PendingDepositsRoot(state.pendingDeposits)
	if err != nil {
		return errors.Wrap(err, "could not compute pending balance deposits merkleization")
	}
	fieldRoots[types.PendingDeposits.RealPosition()] = pbdRoot[:]

	// PendingPartialWithdrawals root.
	ppwRoot, err := stateutil.PendingPartialWithdrawalsRoot(state.pendingPartialWithdrawals)
	if err != nil {
		return errors.Wrap(err, "could not compute pending partial withdrawals merkleization")
	}
	fieldRoots[types.PendingPartialWithdrawals.RealPosition()] = ppwRoot[:]

	// PendingConsolidations root.
	pcRoot, err := stateutil.PendingConsolidationsRoot(state.pendingConsolidations)
	if err != nil {
		return errors.Wrap(err, "could not compute pending consolidations merkleization")
	}
	fieldRoots[types.PendingConsolidations.RealPosition()] = pcRoot[:]

	return nil
}

// computeFuluRoots computes field roots for Fulu version.
func computeFuluRoots(state *BeaconState, fieldRoots [][]byte) error {
	if state.version < version.Fulu {
		return nil
	}

	// Proposer lookahead root.
	proposerLookaheadRoot, err := stateutil.ProposerLookaheadRoot(state.proposerLookahead)
	if err != nil {
		return errors.Wrap(err, "could not compute proposer lookahead merkleization")
	}
	fieldRoots[types.ProposerLookahead.RealPosition()] = proposerLookaheadRoot[:]

	return nil
}

// ComputeFieldRootsWithHasher hashes the provided state and returns its respective field roots.
func ComputeFieldRootsWithHasher(ctx context.Context, state *BeaconState) ([][]byte, error) {
	ctx, span := trace.StartSpan(ctx, "ComputeFieldRootsWithHasher")
	defer span.End()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if state == nil {
		return nil, errors.New("nil state")
	}

	fieldRoots, err := initializeFieldRoots(state)
	if err != nil {
		return nil, err
	}

	if err := computeBasicFieldRoots(state, fieldRoots); err != nil {
		return nil, err
	}

	if err := computeArrayFieldRoots(state, fieldRoots); err != nil {
		return nil, err
	}

	if err := computePhase0SpecificRoots(state, fieldRoots); err != nil {
		return nil, err
	}

	if err := computeAltairPlusRoots(state, fieldRoots); err != nil {
		return nil, err
	}

	if err := computeCheckpointFieldRoots(state, fieldRoots); err != nil {
		return nil, err
	}

	if err := computeExecutionPayloadRoots(state, fieldRoots); err != nil {
		return nil, err
	}

	if err := computeCapellaRoots(state, fieldRoots); err != nil {
		return nil, err
	}

	if err := computeElectraRoots(state, fieldRoots); err != nil {
		return nil, err
	}

	if err := computeFuluRoots(state, fieldRoots); err != nil {
		return nil, err
	}
	return fieldRoots, nil
}
