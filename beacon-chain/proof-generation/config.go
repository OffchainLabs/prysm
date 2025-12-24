package proofgeneration

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/execproof"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

type Config struct {
	StateNotifier statefeed.Notifier
	ProofTypes    []primitives.ExecutionProofId
	Broadcaster   p2p.Broadcaster
	TimeFetcher   blockchain.TimeFetcher
	ExecProofPool execproof.PoolManager
}
