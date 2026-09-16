package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Result labels for validatorPayloadAttestationSubmissionTotal.
const (
	payloadAttestationSuccess            = "success"
	payloadAttestationFailed             = "failed"
	payloadAttestationSkippedNoBlock     = "skipped_no_block"
	payloadAttestationSkippedUnavailable = "skipped_unavailable"

	// Outcome label for validatorPayloadAttestationRetryTotal.
	payloadAttestationRecovered = "recovered"
)

// payloadAttestationDataWithRetry requests the payload attestation data for slot, asking
// once more at the payload attestation deadline when the first request fails before it.
// It reports whether a second request was made.
func (v *validator) payloadAttestationDataWithRetry(ctx context.Context, slot primitives.Slot) (*ethpb.PayloadAttestationData, bool, error) {
	component := params.BeaconConfig().PayloadAttestationDueBPS
	// Sampled before the request so a response crossing the deadline still retries.
	askedBeforeDeadline := v.beforeSlotComponent(slot, component)

	data, err := v.validatorClient.PayloadAttestationData(ctx, slot)
	if err == nil {
		return data, false, nil
	}
	if !askedBeforeDeadline {
		return nil, false, err
	}

	log.WithField("slot", slot).WithError(err).
		Debug("Payload attestation data not final yet, asking again at the deadline")

	v.waitUntilSlotComponent(ctx, slot, component)
	// waitUntilSlotComponent returns silently on cancellation, so check before retrying.
	if ctx.Err() != nil {
		return nil, true, errors.Wrap(err, "context canceled while waiting for the payload attestation deadline")
	}

	data, err = v.validatorClient.PayloadAttestationData(ctx, slot)
	return data, true, err
}

// payloadAttestationRetryOutcome labels the result of a retried request.
func payloadAttestationRetryOutcome(err error) string {
	if err == nil {
		return payloadAttestationRecovered
	}
	return payloadAttestationDataFailure(err)
}

// payloadAttestationDataFailure maps a PayloadAttestationData failure to its submission
// metric label, covering both the gRPC status code and the beacon API HTTP status.
// errors.Is is used rather than errors.As because a multi-node read joins several
// failures and only the former walks the whole tree.
func payloadAttestationDataFailure(err error) string {
	code := status.Code(errors.Cause(err))

	if code == codes.Unavailable ||
		errors.Is(err, &httputil.DefaultJsonError{Code: http.StatusServiceUnavailable}) {
		return payloadAttestationSkippedUnavailable
	}

	if code == codes.NotFound ||
		errors.Is(err, &httputil.DefaultJsonError{Code: http.StatusNoContent}) ||
		errors.Is(err, &httputil.DefaultJsonError{Code: http.StatusNotFound}) {
		return payloadAttestationSkippedNoBlock
	}

	return payloadAttestationFailed
}

// SubmitPayloadAttestation submits a payload attestation message for a PTC member.
func (v *validator) SubmitPayloadAttestation(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitPayloadAttestation")
	defer span.End()
	span.SetAttributes(trace.StringAttribute("validator", fmt.Sprintf("%#x", pubKey)))

	if slots.ToEpoch(slot) < params.BeaconConfig().GloasForkEpoch {
		return
	}

	v.waitForPayloadAvailableOrDeadline(ctx, slot)

	ctx, err := v.withPayloadHeadHint(ctx, slot)
	if err != nil {
		validatorPayloadAttestationSubmissionTotal.WithLabelValues(payloadAttestationFailed).Inc()
		log.WithField("slot", slot).WithError(err).Error("Could not attach freshness hint")
		tracing.AnnotateError(span, err)
		return
	}

	data, retried, err := v.payloadAttestationDataWithRetry(ctx, slot)
	if retried {
		validatorPayloadAttestationRetryTotal.WithLabelValues(payloadAttestationRetryOutcome(err)).Inc()
	}
	if err != nil {
		result := payloadAttestationDataFailure(err)
		validatorPayloadAttestationSubmissionTotal.WithLabelValues(result).Inc()
		tracing.AnnotateError(span, err)

		fields := logrus.Fields{"slot": slot, "retried": retried}
		switch result {
		case payloadAttestationSkippedUnavailable:
			log.WithFields(fields).WithError(err).Info("Skipping payload attestation: data unavailable")
		case payloadAttestationSkippedNoBlock:
			log.WithFields(fields).WithError(err).Info("Skipping payload attestation: no block for slot")
		default:
			log.WithFields(fields).WithError(err).Error("Could not request payload attestation data")
		}
		return
	}

	d, err := v.domainData(ctx, slots.ToEpoch(slot), params.BeaconConfig().DomainPTCAttester[:])
	if err != nil {
		validatorPayloadAttestationSubmissionTotal.WithLabelValues(payloadAttestationFailed).Inc()
		log.WithError(err).Error("Could not get PTC attester domain data")
		return
	}

	r, err := signing.ComputeSigningRoot(data, d.SignatureDomain)
	if err != nil {
		validatorPayloadAttestationSubmissionTotal.WithLabelValues(payloadAttestationFailed).Inc()
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
		validatorPayloadAttestationSubmissionTotal.WithLabelValues(payloadAttestationFailed).Inc()
		log.WithError(err).Error("Could not sign payload attestation")
		return
	}

	duty, err := v.duty(pubKey)
	if err != nil {
		validatorPayloadAttestationSubmissionTotal.WithLabelValues(payloadAttestationFailed).Inc()
		log.WithError(err).Error("Could not fetch validator assignment")
		return
	}

	msg := &ethpb.PayloadAttestationMessage{
		ValidatorIndex: duty.ValidatorIndex,
		Data:           data,
		Signature:      sig.Marshal(),
	}
	if _, err := v.validatorClient.SubmitPayloadAttestation(ctx, msg); err != nil {
		validatorPayloadAttestationSubmissionTotal.WithLabelValues(payloadAttestationFailed).Inc()
		log.WithError(err).Error("Could not submit payload attestation")
		return
	}
	validatorPayloadAttestationSubmissionTotal.WithLabelValues(payloadAttestationSuccess).Inc()

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
	}).Debug("Submitted new payload attestation")
	v.saveSubmittedPayloadAtt(data, duty.ValidatorIndex)
}
