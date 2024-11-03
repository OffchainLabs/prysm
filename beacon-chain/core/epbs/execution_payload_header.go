package epbs

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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
