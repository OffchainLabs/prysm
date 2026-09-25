package p2p

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
)

// ExecutionProofAware is the EIP-8025 `eproof` ENR entry. A node is execution
// proof-aware when the entry is present and non-zero, meaning it subscribes to
// the `execution_proof` gossip topic and applies its validation rules.
type ExecutionProofAware uint8

// ENRKey implements the enr.Entry interface.
func (ExecutionProofAware) ENRKey() string {
	return params.BeaconNetworkConfig().ExecutionProofKey
}

// isExecutionProofAware reports whether a discovered node advertises
// execution-proof awareness.
func isExecutionProofAware(node *enode.Node) bool {
	var aware ExecutionProofAware
	if err := node.Record().Load(enr.WithEntry(aware.ENRKey(), &aware)); err != nil {
		return false
	}
	return aware != 0
}
