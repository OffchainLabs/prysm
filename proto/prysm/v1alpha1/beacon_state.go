package eth

import (
	"slices"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

// Copy deep-copies the state.
func (st *BeaconState) Copy() *BeaconState {
	if st == nil {
		return nil
	}
	cp := *st
	cp.GenesisValidatorsRoot = bytesutil.SafeCopyBytes(st.GenesisValidatorsRoot)
	cp.Fork = st.Fork.Copy()
	cp.LatestBlockHeader = st.LatestBlockHeader.Copy()
	cp.BlockRoots = bytesutil.SafeCopy2dBytes(st.BlockRoots)
	cp.StateRoots = bytesutil.SafeCopy2dBytes(st.StateRoots)
	cp.HistoricalRoots = bytesutil.SafeCopy2dBytes(st.HistoricalRoots)
	cp.Eth1Data = st.Eth1Data.Copy()
	cp.Eth1DataVotes = CopySlice(st.Eth1DataVotes)
	cp.Validators = CopySlice(st.Validators)
	cp.Balances = slices.Clone(st.Balances)
	cp.RandaoMixes = bytesutil.SafeCopy2dBytes(st.RandaoMixes)
	cp.Slashings = slices.Clone(st.Slashings)
	cp.PreviousEpochAttestations = CopySlice(st.PreviousEpochAttestations)
	cp.CurrentEpochAttestations = CopySlice(st.CurrentEpochAttestations)
	cp.JustificationBits = bytesutil.SafeCopyBytes(st.JustificationBits)
	cp.PreviousJustifiedCheckpoint = st.PreviousJustifiedCheckpoint.Copy()
	cp.CurrentJustifiedCheckpoint = st.CurrentJustifiedCheckpoint.Copy()
	cp.FinalizedCheckpoint = st.FinalizedCheckpoint.Copy()
	return &cp
}

// Copy deep-copies the state.
func (st *BeaconStateAltair) Copy() *BeaconStateAltair {
	if st == nil {
		return nil
	}
	cp := *st
	cp.GenesisValidatorsRoot = bytesutil.SafeCopyBytes(st.GenesisValidatorsRoot)
	cp.Fork = st.Fork.Copy()
	cp.LatestBlockHeader = st.LatestBlockHeader.Copy()
	cp.BlockRoots = bytesutil.SafeCopy2dBytes(st.BlockRoots)
	cp.StateRoots = bytesutil.SafeCopy2dBytes(st.StateRoots)
	cp.HistoricalRoots = bytesutil.SafeCopy2dBytes(st.HistoricalRoots)
	cp.Eth1Data = st.Eth1Data.Copy()
	cp.Eth1DataVotes = CopySlice(st.Eth1DataVotes)
	cp.Validators = CopySlice(st.Validators)
	cp.Balances = slices.Clone(st.Balances)
	cp.RandaoMixes = bytesutil.SafeCopy2dBytes(st.RandaoMixes)
	cp.Slashings = slices.Clone(st.Slashings)
	cp.PreviousEpochParticipation = bytesutil.SafeCopyBytes(st.PreviousEpochParticipation)
	cp.CurrentEpochParticipation = bytesutil.SafeCopyBytes(st.CurrentEpochParticipation)
	cp.JustificationBits = bytesutil.SafeCopyBytes(st.JustificationBits)
	cp.PreviousJustifiedCheckpoint = st.PreviousJustifiedCheckpoint.Copy()
	cp.CurrentJustifiedCheckpoint = st.CurrentJustifiedCheckpoint.Copy()
	cp.FinalizedCheckpoint = st.FinalizedCheckpoint.Copy()
	cp.InactivityScores = slices.Clone(st.InactivityScores)
	cp.CurrentSyncCommittee = st.CurrentSyncCommittee.Copy()
	cp.NextSyncCommittee = st.NextSyncCommittee.Copy()
	return &cp
}

// Copy deep-copies the state.
func (st *BeaconStateBellatrix) Copy() *BeaconStateBellatrix {
	if st == nil {
		return nil
	}
	cp := *st
	cp.GenesisValidatorsRoot = bytesutil.SafeCopyBytes(st.GenesisValidatorsRoot)
	cp.Fork = st.Fork.Copy()
	cp.LatestBlockHeader = st.LatestBlockHeader.Copy()
	cp.BlockRoots = bytesutil.SafeCopy2dBytes(st.BlockRoots)
	cp.StateRoots = bytesutil.SafeCopy2dBytes(st.StateRoots)
	cp.HistoricalRoots = bytesutil.SafeCopy2dBytes(st.HistoricalRoots)
	cp.Eth1Data = st.Eth1Data.Copy()
	cp.Eth1DataVotes = CopySlice(st.Eth1DataVotes)
	cp.Validators = CopySlice(st.Validators)
	cp.Balances = slices.Clone(st.Balances)
	cp.RandaoMixes = bytesutil.SafeCopy2dBytes(st.RandaoMixes)
	cp.Slashings = slices.Clone(st.Slashings)
	cp.PreviousEpochParticipation = bytesutil.SafeCopyBytes(st.PreviousEpochParticipation)
	cp.CurrentEpochParticipation = bytesutil.SafeCopyBytes(st.CurrentEpochParticipation)
	cp.JustificationBits = bytesutil.SafeCopyBytes(st.JustificationBits)
	cp.PreviousJustifiedCheckpoint = st.PreviousJustifiedCheckpoint.Copy()
	cp.CurrentJustifiedCheckpoint = st.CurrentJustifiedCheckpoint.Copy()
	cp.FinalizedCheckpoint = st.FinalizedCheckpoint.Copy()
	cp.InactivityScores = slices.Clone(st.InactivityScores)
	cp.CurrentSyncCommittee = st.CurrentSyncCommittee.Copy()
	cp.NextSyncCommittee = st.NextSyncCommittee.Copy()
	cp.LatestExecutionPayloadHeader = st.LatestExecutionPayloadHeader.Copy()
	return &cp
}

// Copy deep-copies the state.
func (st *BeaconStateCapella) Copy() *BeaconStateCapella {
	if st == nil {
		return nil
	}
	cp := *st
	cp.GenesisValidatorsRoot = bytesutil.SafeCopyBytes(st.GenesisValidatorsRoot)
	cp.Fork = st.Fork.Copy()
	cp.LatestBlockHeader = st.LatestBlockHeader.Copy()
	cp.BlockRoots = bytesutil.SafeCopy2dBytes(st.BlockRoots)
	cp.StateRoots = bytesutil.SafeCopy2dBytes(st.StateRoots)
	cp.HistoricalRoots = bytesutil.SafeCopy2dBytes(st.HistoricalRoots)
	cp.Eth1Data = st.Eth1Data.Copy()
	cp.Eth1DataVotes = CopySlice(st.Eth1DataVotes)
	cp.Validators = CopySlice(st.Validators)
	cp.Balances = slices.Clone(st.Balances)
	cp.RandaoMixes = bytesutil.SafeCopy2dBytes(st.RandaoMixes)
	cp.Slashings = slices.Clone(st.Slashings)
	cp.PreviousEpochParticipation = bytesutil.SafeCopyBytes(st.PreviousEpochParticipation)
	cp.CurrentEpochParticipation = bytesutil.SafeCopyBytes(st.CurrentEpochParticipation)
	cp.JustificationBits = bytesutil.SafeCopyBytes(st.JustificationBits)
	cp.PreviousJustifiedCheckpoint = st.PreviousJustifiedCheckpoint.Copy()
	cp.CurrentJustifiedCheckpoint = st.CurrentJustifiedCheckpoint.Copy()
	cp.FinalizedCheckpoint = st.FinalizedCheckpoint.Copy()
	cp.InactivityScores = slices.Clone(st.InactivityScores)
	cp.CurrentSyncCommittee = st.CurrentSyncCommittee.Copy()
	cp.NextSyncCommittee = st.NextSyncCommittee.Copy()
	cp.LatestExecutionPayloadHeader = st.LatestExecutionPayloadHeader.Copy()
	cp.HistoricalSummaries = CopySlice(st.HistoricalSummaries)
	return &cp
}

// Copy deep-copies the state.
func (st *BeaconStateDeneb) Copy() *BeaconStateDeneb {
	if st == nil {
		return nil
	}
	cp := *st
	cp.GenesisValidatorsRoot = bytesutil.SafeCopyBytes(st.GenesisValidatorsRoot)
	cp.Fork = st.Fork.Copy()
	cp.LatestBlockHeader = st.LatestBlockHeader.Copy()
	cp.BlockRoots = bytesutil.SafeCopy2dBytes(st.BlockRoots)
	cp.StateRoots = bytesutil.SafeCopy2dBytes(st.StateRoots)
	cp.HistoricalRoots = bytesutil.SafeCopy2dBytes(st.HistoricalRoots)
	cp.Eth1Data = st.Eth1Data.Copy()
	cp.Eth1DataVotes = CopySlice(st.Eth1DataVotes)
	cp.Validators = CopySlice(st.Validators)
	cp.Balances = slices.Clone(st.Balances)
	cp.RandaoMixes = bytesutil.SafeCopy2dBytes(st.RandaoMixes)
	cp.Slashings = slices.Clone(st.Slashings)
	cp.PreviousEpochParticipation = bytesutil.SafeCopyBytes(st.PreviousEpochParticipation)
	cp.CurrentEpochParticipation = bytesutil.SafeCopyBytes(st.CurrentEpochParticipation)
	cp.JustificationBits = bytesutil.SafeCopyBytes(st.JustificationBits)
	cp.PreviousJustifiedCheckpoint = st.PreviousJustifiedCheckpoint.Copy()
	cp.CurrentJustifiedCheckpoint = st.CurrentJustifiedCheckpoint.Copy()
	cp.FinalizedCheckpoint = st.FinalizedCheckpoint.Copy()
	cp.InactivityScores = slices.Clone(st.InactivityScores)
	cp.CurrentSyncCommittee = st.CurrentSyncCommittee.Copy()
	cp.NextSyncCommittee = st.NextSyncCommittee.Copy()
	cp.LatestExecutionPayloadHeader = st.LatestExecutionPayloadHeader.Copy()
	cp.HistoricalSummaries = CopySlice(st.HistoricalSummaries)
	return &cp
}

// Copy deep-copies the state.
func (st *BeaconStateElectra) Copy() *BeaconStateElectra {
	if st == nil {
		return nil
	}
	cp := *st
	cp.GenesisValidatorsRoot = bytesutil.SafeCopyBytes(st.GenesisValidatorsRoot)
	cp.Fork = st.Fork.Copy()
	cp.LatestBlockHeader = st.LatestBlockHeader.Copy()
	cp.BlockRoots = bytesutil.SafeCopy2dBytes(st.BlockRoots)
	cp.StateRoots = bytesutil.SafeCopy2dBytes(st.StateRoots)
	cp.HistoricalRoots = bytesutil.SafeCopy2dBytes(st.HistoricalRoots)
	cp.Eth1Data = st.Eth1Data.Copy()
	cp.Eth1DataVotes = CopySlice(st.Eth1DataVotes)
	cp.Validators = CopySlice(st.Validators)
	cp.Balances = slices.Clone(st.Balances)
	cp.RandaoMixes = bytesutil.SafeCopy2dBytes(st.RandaoMixes)
	cp.Slashings = slices.Clone(st.Slashings)
	cp.PreviousEpochParticipation = bytesutil.SafeCopyBytes(st.PreviousEpochParticipation)
	cp.CurrentEpochParticipation = bytesutil.SafeCopyBytes(st.CurrentEpochParticipation)
	cp.JustificationBits = bytesutil.SafeCopyBytes(st.JustificationBits)
	cp.PreviousJustifiedCheckpoint = st.PreviousJustifiedCheckpoint.Copy()
	cp.CurrentJustifiedCheckpoint = st.CurrentJustifiedCheckpoint.Copy()
	cp.FinalizedCheckpoint = st.FinalizedCheckpoint.Copy()
	cp.InactivityScores = slices.Clone(st.InactivityScores)
	cp.CurrentSyncCommittee = st.CurrentSyncCommittee.Copy()
	cp.NextSyncCommittee = st.NextSyncCommittee.Copy()
	cp.LatestExecutionPayloadHeader = st.LatestExecutionPayloadHeader.Copy()
	cp.HistoricalSummaries = CopySlice(st.HistoricalSummaries)
	cp.PendingDeposits = CopySlice(st.PendingDeposits)
	cp.PendingPartialWithdrawals = CopySlice(st.PendingPartialWithdrawals)
	cp.PendingConsolidations = CopySlice(st.PendingConsolidations)
	return &cp
}

// Copy deep-copies the state.
func (st *BeaconStateFulu) Copy() *BeaconStateFulu {
	if st == nil {
		return nil
	}
	cp := *st
	cp.GenesisValidatorsRoot = bytesutil.SafeCopyBytes(st.GenesisValidatorsRoot)
	cp.Fork = st.Fork.Copy()
	cp.LatestBlockHeader = st.LatestBlockHeader.Copy()
	cp.BlockRoots = bytesutil.SafeCopy2dBytes(st.BlockRoots)
	cp.StateRoots = bytesutil.SafeCopy2dBytes(st.StateRoots)
	cp.HistoricalRoots = bytesutil.SafeCopy2dBytes(st.HistoricalRoots)
	cp.Eth1Data = st.Eth1Data.Copy()
	cp.Eth1DataVotes = CopySlice(st.Eth1DataVotes)
	cp.Validators = CopySlice(st.Validators)
	cp.Balances = slices.Clone(st.Balances)
	cp.RandaoMixes = bytesutil.SafeCopy2dBytes(st.RandaoMixes)
	cp.Slashings = slices.Clone(st.Slashings)
	cp.PreviousEpochParticipation = bytesutil.SafeCopyBytes(st.PreviousEpochParticipation)
	cp.CurrentEpochParticipation = bytesutil.SafeCopyBytes(st.CurrentEpochParticipation)
	cp.JustificationBits = bytesutil.SafeCopyBytes(st.JustificationBits)
	cp.PreviousJustifiedCheckpoint = st.PreviousJustifiedCheckpoint.Copy()
	cp.CurrentJustifiedCheckpoint = st.CurrentJustifiedCheckpoint.Copy()
	cp.FinalizedCheckpoint = st.FinalizedCheckpoint.Copy()
	cp.InactivityScores = slices.Clone(st.InactivityScores)
	cp.CurrentSyncCommittee = st.CurrentSyncCommittee.Copy()
	cp.NextSyncCommittee = st.NextSyncCommittee.Copy()
	cp.LatestExecutionPayloadHeader = st.LatestExecutionPayloadHeader.Copy()
	cp.HistoricalSummaries = CopySlice(st.HistoricalSummaries)
	cp.PendingDeposits = CopySlice(st.PendingDeposits)
	cp.PendingPartialWithdrawals = CopySlice(st.PendingPartialWithdrawals)
	cp.PendingConsolidations = CopySlice(st.PendingConsolidations)
	cp.ProposerLookahead = slices.Clone(st.ProposerLookahead)
	return &cp
}

// Copy deep-copies the state.
func (st *BeaconStateGloas) Copy() *BeaconStateGloas {
	if st == nil {
		return nil
	}
	cp := *st
	cp.GenesisValidatorsRoot = bytesutil.SafeCopyBytes(st.GenesisValidatorsRoot)
	cp.Fork = st.Fork.Copy()
	cp.LatestBlockHeader = st.LatestBlockHeader.Copy()
	cp.BlockRoots = bytesutil.SafeCopy2dBytes(st.BlockRoots)
	cp.StateRoots = bytesutil.SafeCopy2dBytes(st.StateRoots)
	cp.HistoricalRoots = bytesutil.SafeCopy2dBytes(st.HistoricalRoots)
	cp.Eth1Data = st.Eth1Data.Copy()
	cp.Eth1DataVotes = CopySlice(st.Eth1DataVotes)
	cp.Validators = CopySlice(st.Validators)
	cp.Balances = slices.Clone(st.Balances)
	cp.RandaoMixes = bytesutil.SafeCopy2dBytes(st.RandaoMixes)
	cp.Slashings = slices.Clone(st.Slashings)
	cp.PreviousEpochParticipation = bytesutil.SafeCopyBytes(st.PreviousEpochParticipation)
	cp.CurrentEpochParticipation = bytesutil.SafeCopyBytes(st.CurrentEpochParticipation)
	cp.JustificationBits = bytesutil.SafeCopyBytes(st.JustificationBits)
	cp.PreviousJustifiedCheckpoint = st.PreviousJustifiedCheckpoint.Copy()
	cp.CurrentJustifiedCheckpoint = st.CurrentJustifiedCheckpoint.Copy()
	cp.FinalizedCheckpoint = st.FinalizedCheckpoint.Copy()
	cp.InactivityScores = slices.Clone(st.InactivityScores)
	cp.CurrentSyncCommittee = st.CurrentSyncCommittee.Copy()
	cp.NextSyncCommittee = st.NextSyncCommittee.Copy()
	cp.LatestBlockHash = bytesutil.SafeCopyBytes(st.LatestBlockHash)
	cp.HistoricalSummaries = CopySlice(st.HistoricalSummaries)
	cp.PendingDeposits = CopySlice(st.PendingDeposits)
	cp.PendingPartialWithdrawals = CopySlice(st.PendingPartialWithdrawals)
	cp.PendingConsolidations = CopySlice(st.PendingConsolidations)
	cp.ProposerLookahead = slices.Clone(st.ProposerLookahead)
	cp.Builders = CopySlice(st.Builders)
	cp.ExecutionPayloadAvailability = bytesutil.SafeCopyBytes(st.ExecutionPayloadAvailability)
	cp.BuilderPendingPayments = CopySlice(st.BuilderPendingPayments)
	cp.BuilderPendingWithdrawals = CopySlice(st.BuilderPendingWithdrawals)
	cp.LatestExecutionPayloadBid = st.LatestExecutionPayloadBid.Copy()
	cp.PayloadExpectedWithdrawals = CopySlice(st.PayloadExpectedWithdrawals)
	cp.PtcWindow = CopyPTCWindow(st.PtcWindow)
	return &cp
}
