package client

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func (v *validator) signExecutionPayloadEnvelope(ctx context.Context, p *enginev1.ExecutionPayloadEnvelope, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	// Get domain data
	currentSlot := slots.CurrentSlot(v.genesisTime)
	epoch := slots.ToEpoch(currentSlot)
	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainBeaconBuilder[:])
	if err != nil {
		return nil, errors.Wrap(err, domainDataErr)
	}
	if domain == nil {
		return nil, errors.New(domainDataErr)
	}

	// Compute signing root
	signingRoot, err := signing.ComputeSigningRoot(p, domain.SignatureDomain)
	if err != nil {
		return nil, errors.Wrap(err, signingRootErr)
	}

	// Create signature request
	signReq := &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     signingRoot[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          &validatorpb.SignRequest_ExecutionPayloadEnvelope{ExecutionPayloadEnvelope: p},
		SigningSlot:     currentSlot,
	}

	// Sign the execution payload envelope.
	m, err := v.Keymanager()
	if err != nil {
		return nil, errors.Wrap(err, "could not get key manager")
	}
	sig, err := m.Sign(ctx, signReq)
	if err != nil {
		return nil, errors.Wrap(err, "could not sign payload attestation")
	}

	return sig.Marshal(), nil
}
