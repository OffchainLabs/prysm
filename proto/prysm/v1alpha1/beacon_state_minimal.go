//go:build minimal

package eth

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// BeaconStateAltair is the Altair SSZ state.
type BeaconStateAltair struct {
	GenesisTime                 uint64
	GenesisValidatorsRoot       []byte `ssz-size:"32"`
	Slot                        primitives.Slot
	Fork                        *Fork
	LatestBlockHeader           *BeaconBlockHeader
	BlockRoots                  [][]byte `ssz-size:"64,32"`
	StateRoots                  [][]byte `ssz-size:"64,32"`
	HistoricalRoots             [][]byte `ssz-size:"?,32" ssz-max:"16777216"`
	Eth1Data                    *Eth1Data
	Eth1DataVotes               []*Eth1Data `ssz-max:"32"`
	Eth1DepositIndex            uint64
	Validators                  []*Validator        `ssz-max:"1099511627776"`
	Balances                    []uint64            `ssz-max:"1099511627776"`
	RandaoMixes                 [][]byte            `ssz-size:"64,32"`
	Slashings                   []uint64            `ssz-size:"64"`
	PreviousEpochParticipation  []byte              `ssz-max:"1099511627776"`
	CurrentEpochParticipation   []byte              `ssz-max:"1099511627776"`
	JustificationBits           bitfield.Bitvector4 `ssz-size:"1"`
	PreviousJustifiedCheckpoint *Checkpoint
	CurrentJustifiedCheckpoint  *Checkpoint
	FinalizedCheckpoint         *Checkpoint
	InactivityScores            []uint64 `ssz-max:"1099511627776"`
	CurrentSyncCommittee        *SyncCommittee
	NextSyncCommittee           *SyncCommittee
}
