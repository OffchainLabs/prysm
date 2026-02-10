// Package prover defines handlers for the prover API endpoints.
package prover

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
)

// Server defines a server implementation for the prover API endpoints.
type Server struct {
	Broadcaster p2p.Broadcaster
}
