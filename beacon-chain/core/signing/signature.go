package signing

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

var ErrNilRegistration = errors.New("nil signed registration")

// VerifyRegistrationSignature verifies the signature of a validator's registration.
func VerifyRegistrationSignature(
	sr *ethpb.SignedValidatorRegistrationV1,
) error {
	if sr == nil || sr.Message == nil {
		return ErrNilRegistration
	}

	d := params.BeaconConfig().DomainApplicationBuilder
	// Per spec, we want the fork version and genesis validator to be nil.
	// Which is genesis value and zero by default.
	sd, err := ComputeDomain(
		d,
		nil, /* fork version */
		nil /* genesis val root */)
	if err != nil {
		return err
	}

	if err := VerifySigningRoot(sr.Message, sr.Message.Pubkey, sr.Signature, sd); err != nil {
		return ErrSigFailedToVerify
	}
	return nil
}
