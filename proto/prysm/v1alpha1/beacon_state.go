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
