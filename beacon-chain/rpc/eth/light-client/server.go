package lightclient

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	lightClient "github.com/OffchainLabs/prysm/v7/beacon-chain/light-client"
)

type Server struct {
	LCStore     *lightClient.Store
	HeadFetcher blockchain.HeadFetcher
}
