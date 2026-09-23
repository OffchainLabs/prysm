package client

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"sync/atomic"
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
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	payloadAttestationReadGrace    = 500 * time.Millisecond
	payloadAttestationPollInterval = 50 * time.Millisecond
)

// Result labels for validatorPayloadAttestationSubmissionTotal.
const (
	payloadAttestationSuccess            = "success"
	payloadAttestationFailed             = "failed"
	payloadAttestationSkippedNoBlock     = "skipped_no_block"
	payloadAttestationSkippedUnavailable = "skipped_unavailable"

	// Outcome labels for validatorPayloadAttestationRetryTotal.
	payloadAttestationRecovered = "recovered"
	payloadAttestationFallback  = "fallback"
)

// gRPC and REST share this read budget, root preference and fallback policy.
func (v *validator) payloadAttestationDataWithRetry(ctx context.Context, slot primitives.Slot) (*ethpb.PayloadAttestationData, bool, error) {
	deadline, err := v.slotComponentDeadline(slot, params.BeaconConfig().PayloadAttestationDueBPS)
	if err != nil {
		return nil, false, err
	}
	if now := time.Now(); deadline.Before(now) {
		deadline = now
	}
	readCtx, cancel := context.WithDeadline(ctx, deadline.Add(payloadAttestationReadGrace))
	defer cancel()
	var retried atomic.Bool
	readCtx = iface.WithPayloadAttestationRetry(readCtx, payloadAttestationPollInterval, func() { retried.Store(true) })

	var fallback *ethpb.PayloadAttestationData
	var lastErr error
	for attempts := 0; readCtx.Err() == nil; attempts++ {
		if attempts > 0 {
			retried.Store(true)
		}
		data, err := v.validatorClient.PayloadAttestationData(readCtx, slot)
		if ctx.Err() != nil {
			return nil, retried.Load(), stderrors.Join(lastErr, err, ctx.Err())
		}
		if err == nil {
			if payloadAttestationMatchesHint(ctx, data) {
				return data, retried.Load(), nil
			}
			fallback = data
		} else if readCtx.Err() == nil || lastErr == nil {
			lastErr = err
		}
		select {
		case <-readCtx.Done():
		case <-time.After(payloadAttestationPollInterval):
		}
	}
	if ctx.Err() != nil {
		return nil, retried.Load(), stderrors.Join(lastErr, ctx.Err())
	}
	if fallback != nil {
		return fallback, retried.Load(), nil
	}
	if lastErr == nil {
		lastErr = readCtx.Err()
	}
	return nil, retried.Load(), lastErr
}

func payloadAttestationMatchesHint(ctx context.Context, data *ethpb.PayloadAttestationData) bool {
	hint, ok := iface.FromContext(ctx)
	if !ok {
		return true
	}
	head, known := hint.Head()
	return !known || bytesutil.ToBytes32(data.BeaconBlockRoot) == head.Root
}

// payloadAttestationRetryOutcome labels the result of a retried request.
func payloadAttestationRetryOutcome(ctx context.Context, data *ethpb.PayloadAttestationData, err error) string {
	if err != nil {
		return payloadAttestationDataFailure(err)
	}
	if !payloadAttestationMatchesHint(ctx, data) {
		return payloadAttestationFallback
	}
	return payloadAttestationRecovered
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
		validatorPayloadAttestationRetryTotal.WithLabelValues(payloadAttestationRetryOutcome(ctx, data, err)).Inc()
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
