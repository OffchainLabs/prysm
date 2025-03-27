package client

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	validatorpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

func (v *validator) signExecutionPayloadHeader(ctx context.Context, p *enginev1.ExecutionPayloadHeaderEPBS, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	// Get domain data
	epoch := slots.ToEpoch(p.Slot)
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
		Object:          &validatorpb.SignRequest_ExecutionPayloadHeaderEpbs{ExecutionPayloadHeaderEpbs: p},
		SigningSlot:     p.Slot,
	}

	// Sign the payload attestation data
	m, err := v.Keymanager()
	if err != nil {
		return nil, errors.Wrap(err, "could not get key manager")
	}
	sig, err := m.Sign(ctx, signReq)
	if err != nil {
		return nil, errors.Wrap(err, "could not sign payload attestation")
	}

	// Marshal the signature into bytes
	return sig.Marshal(), nil
}
