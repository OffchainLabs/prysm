package lightclient

import (
	lightClient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
)

type Server struct {
	LCStore *lightClient.Store
}
