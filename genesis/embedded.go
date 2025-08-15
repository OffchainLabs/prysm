package genesis

import (
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/genesis/internal/embedded"
)

var embeddedGenesisData map[string]GenesisData

func init() {
	embeddedGenesisData = make(map[string]GenesisData)
	embeddedGenesisData[params.MainnetName] = GenesisData{
		ValidatorsRoot: [32]byte{75, 54, 61, 185, 78, 40, 97, 32, 215, 110, 185, 5, 52, 15, 221, 78, 84, 191, 233, 240, 107, 243, 63, 246, 207, 90, 210, 127, 81, 27, 254, 149},
		Time:           time.Unix(1606824023, 0),
		embeddedBytes: func() ([]byte, error) {
			return embedded.BytesByName(params.MainnetName)
		},
		embeddedState: func() (state.BeaconState, error) {
			return embedded.ByName(params.MainnetName)
		},
	}
}
