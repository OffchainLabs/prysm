package cache

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

// ProposerIndices defines the cached struct for proposer indices.
type ProposerIndices struct {
	BlockRoot       [32]byte
	ProposerIndices []primitives.ValidatorIndex
}
