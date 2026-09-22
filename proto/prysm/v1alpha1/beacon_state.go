package eth

import (
	"slices"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

// Copy deep-copies the state.
func (st *BeaconStateAltair) Copy() *BeaconStateAltair {
	if st == nil {
		return nil
	}
	return &BeaconStateAltair{
		GenesisTime:                 st.GenesisTime,
		GenesisValidatorsRoot:       bytesutil.SafeCopyBytes(st.GenesisValidatorsRoot),
		Slot:                        st.Slot,
		Fork:                        st.Fork.Copy(),
		LatestBlockHeader:           st.LatestBlockHeader.Copy(),
		BlockRoots:                  bytesutil.SafeCopy2dBytes(st.BlockRoots),
		StateRoots:                  bytesutil.SafeCopy2dBytes(st.StateRoots),
		HistoricalRoots:             bytesutil.SafeCopy2dBytes(st.HistoricalRoots),
		Eth1Data:                    st.Eth1Data.Copy(),
		Eth1DataVotes:               CopySlice(st.Eth1DataVotes),
		Eth1DepositIndex:            st.Eth1DepositIndex,
		Validators:                  CopySlice(st.Validators),
		Balances:                    slices.Clone(st.Balances),
		RandaoMixes:                 bytesutil.SafeCopy2dBytes(st.RandaoMixes),
		Slashings:                   slices.Clone(st.Slashings),
		PreviousEpochParticipation:  bytesutil.SafeCopyBytes(st.PreviousEpochParticipation),
		CurrentEpochParticipation:   bytesutil.SafeCopyBytes(st.CurrentEpochParticipation),
		JustificationBits:           bytesutil.SafeCopyBytes(st.JustificationBits),
		PreviousJustifiedCheckpoint: st.PreviousJustifiedCheckpoint.Copy(),
		CurrentJustifiedCheckpoint:  st.CurrentJustifiedCheckpoint.Copy(),
		FinalizedCheckpoint:         st.FinalizedCheckpoint.Copy(),
		InactivityScores:            slices.Clone(st.InactivityScores),
		CurrentSyncCommittee:        st.CurrentSyncCommittee.Copy(),
		NextSyncCommittee:           st.NextSyncCommittee.Copy(),
	}
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
