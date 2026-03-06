package client

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// SubmitPayloadAttestation submits a payload attestation message for a PTC member.
func (v *validator) SubmitPayloadAttestation(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitPayloadAttestation")
	defer span.End()
	span.SetAttributes(trace.StringAttribute("validator", fmt.Sprintf("%#x", pubKey)))

	if slots.ToEpoch(slot) < params.BeaconConfig().GloasForkEpoch {
		return
	}

	v.waitUntilSlotComponent(ctx, slot, params.BeaconConfig().PayloadAttestationDueBPS)

	data, err := v.validatorClient.PayloadAttestationData(ctx, slot)
	if err != nil {
		log.WithError(err).Error("Could not request payload attestation data")
		tracing.AnnotateError(span, err)
		return
	}

	d, err := v.domainData(ctx, slots.ToEpoch(slot), params.BeaconConfig().DomainPTCAttester[:])
	if err != nil {
		log.WithError(err).Error("Could not get PTC attester domain data")
		return
	}

	r, err := signing.ComputeSigningRoot(data, d.SignatureDomain)
	if err != nil {
		log.WithError(err).Error("Could not compute payload attestation signing root")
		return
	}

	sig, err := v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     r[:],
		SignatureDomain: d.SignatureDomain,
		Object: &validatorpb.SignRequest_PayloadAttestationData{
			PayloadAttestationData: data,
		},
		SigningSlot: slot,
	})
	if err != nil {
		log.WithError(err).Error("Could not sign payload attestation")
		return
	}

	duty, err := v.duty(pubKey)
	if err != nil {
		log.WithError(err).Error("Could not fetch validator assignment")
		return
	}

	msg := &ethpb.PayloadAttestationMessage{
		ValidatorIndex: duty.ValidatorIndex,
		Data:           data,
		Signature:      sig.Marshal(),
	}
	if _, err := v.validatorClient.SubmitPayloadAttestation(ctx, msg); err != nil {
		log.WithError(err).Error("Could not submit payload attestation")
		return
	}

	slotTime, err := slots.StartTime(v.genesisTime, slot)
	if err != nil {
		log.WithError(err).Error("Failed to determine slot start time")
	}
	log.WithFields(logrus.Fields{
		"slot":               slot,
		"slotStartTime":      slotTime,
		"timeSinceSlotStart": time.Since(slotTime),
		"blockRoot":          fmt.Sprintf("%#x", bytesutil.Trunc(data.BeaconBlockRoot)),
		"payloadPresent":     data.PayloadPresent,
		"blobDataAvailable":  data.BlobDataAvailable,
		"validatorIndex":     duty.ValidatorIndex,
	}).Info("Submitted new payload attestation")
}
