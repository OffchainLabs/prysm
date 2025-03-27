package epbs

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/network/forks"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

// ValidatePayloadHeaderSignature validates the signature of the execution payload header.
func ValidatePayloadHeaderSignature(st state.ReadOnlyBeaconState, sh interfaces.ROSignedExecutionPayloadHeader) error {
	h, err := sh.Header()
	if err != nil {
		return err
	}
	pubkey := st.PubkeyAtIndex(h.BuilderIndex())
	pub, err := bls.PublicKeyFromBytes(pubkey[:])
	if err != nil {
		return err
	}

	s := sh.Signature()
	sig, err := bls.SignatureFromBytes(s[:])
	if err != nil {
		return err
	}

	currentEpoch := slots.ToEpoch(h.Slot())
	f, err := forks.Fork(currentEpoch)
	if err != nil {
		return err
	}

	domain, err := signing.Domain(f, currentEpoch, params.BeaconConfig().DomainBeaconBuilder, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	root, err := sh.SigningRoot(domain)
	if err != nil {
		return err
	}
	if !sig.Verify(pub, root[:]) {
		return signing.ErrSigFailedToVerify
	}

	return nil
}
