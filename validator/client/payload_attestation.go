package client

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// SubmitPayloadAttestationMessage submits a payload attestation message to the beacon node.
func (v *validator) SubmitPayloadAttestationMessage(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	data, err := v.validatorClient.GetPayloadAttestationData(ctx, &ethpb.GetPayloadAttestationDataRequest{Slot: slot})
	if err != nil {
		log.WithError(err).Error("could not get payload attestation data")
		return
	}

	signature, err := v.signPayloadAttestation(ctx, data, pubKey)
	if err != nil {
		log.WithError(err).Error("could not sign payload attestation")
		return
	}

	index, found := v.pubkeyToValidatorIndex[pubKey]
	if !found {
		log.WithField("pubkey", pubKey).Error("could not find validator index for pubkey")
		return
	}

	message := &ethpb.PayloadAttestationMessage{
		ValidatorIndex: index,
		Data:           data,
		Signature:      signature,
	}

	if _, err := v.validatorClient.SubmitPayloadAttestation(ctx, message); err != nil {
		log.WithError(err).Error("could not submit payload attestation")
	}
}

func (v *validator) signPayloadAttestation(ctx context.Context, p *ethpb.PayloadAttestationData, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	// Get domain data
	epoch := slots.ToEpoch(p.Slot)
	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainPTCAttester[:])
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
		Object:          &validatorpb.SignRequest_PayloadAttestationData{PayloadAttestationData: p},
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

// waitUntilPtcDuty waits until PTC duty time which is defined as 3/4 of slot time.
func (v *validator) waitUntilPtcDuty(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.waitUntilPtcDuty")
	defer span.End()

	startTime := slots.StartTime(v.genesisTime, slot)
	dutyTime := startTime.Add(3 * slots.DivideSlotBy(4))
	wait := prysmTime.Until(dutyTime)
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		tracing.AnnotateError(span, ctx.Err())
		return
	case <-t.C:
		return
	}
}
