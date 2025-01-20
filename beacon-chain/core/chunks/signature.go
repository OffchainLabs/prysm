package chunks

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/network/forks"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

// VerifyChunkSignature verifies the proposer signature of a beacon block chunk.
func VerifyChunkSignature(beaconState state.ReadOnlyBeaconState,
	proposerIndex primitives.ValidatorIndex,
	sig []byte,
	rootFunc func() ([32]byte, error)) error {
	currentEpoch := slots.ToEpoch(beaconState.Slot())
	domain, err := signing.Domain(beaconState.Fork(), currentEpoch, params.BeaconConfig().DomainBeaconProposer, beaconState.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	proposer, err := beaconState.ValidatorAtIndex(proposerIndex)
	if err != nil {
		return err
	}
	proposerPubKey := proposer.PublicKey
	return signing.VerifyBlockSigningRoot(proposerPubKey, sig, domain, rootFunc)
}

// VerifyChunkSignatureUsingCurrentFork verifies the proposer signature of a beacon block chunk. This differs
// from the above method by not using fork data from the state and instead retrieving it
// via the respective epoch.
func VerifyChunkSignatureUsingCurrentFork(beaconState state.ReadOnlyBeaconState, chunk interfaces.ReadOnlyBeaconBlockChunk) error {
	currentEpoch := slots.ToEpoch(chunk.Slot())
	fork, err := forks.Fork(currentEpoch)
	if err != nil {
		return err
	}
	domain, err := signing.Domain(fork, currentEpoch, params.BeaconConfig().DomainBeaconProposer, beaconState.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	proposer, err := beaconState.ValidatorAtIndex(chunk.ProposerIndex())
	if err != nil {
		return err
	}
	proposerPubKey := proposer.PublicKey
	sig := chunk.Signature()
	return signing.VerifyBlockSigningRoot(proposerPubKey, sig[:], domain, func() ([32]byte, error) {
		return chunk.HeaderRoot(), nil
	})
}
