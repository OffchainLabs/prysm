package proofgeneration

import (
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

type Config struct {
	StateNotifier statefeed.Notifier
	ProofTypes    []primitives.ExecutionProofId
}
