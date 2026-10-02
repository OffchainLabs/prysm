// Package prover exposes the beacon-node endpoints an EIP-8025 prover uses.
package prover

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
)

// Server implements the execution proof endpoints of the beacon API.
type Server struct {
	Broadcaster         p2p.Broadcaster
	ExecutionProofCache *cache.ExecutionProofCache
}
