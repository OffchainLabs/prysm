package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SubmitAggregateAndProof submits the validator's signed slot signature to the beacon node
// via gRPC. Beacon node will verify the slot signature and determine if the validator is also
// an aggregator. If yes, then beacon node will broadcast aggregated signature and
// proof on the validator's behalf.
func (v *validator) SubmitAggregateAndProof(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitAggregateAndProof")
	defer span.End()

	span.SetAttributes(trace.StringAttribute("validator", fmt.Sprintf("%#x", pubKey)))
	fmtKey := fmt.Sprintf("%#x", pubKey[:])

	duty, err := v.duty(pubKey)
	if err != nil {
		log.WithError(err).Error("Could not fetch validator assignment")
		if v.emitAccountMetrics {
			ValidatorAggFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	// Avoid sending beacon node duplicated aggregation requests.
	k := validatorSubnetSubscriptionKey(slot, duty.CommitteeIndex)
	v.aggregatedSlotCommitteeIDCacheLock.Lock()
	if v.aggregatedSlotCommitteeIDCache.Contains(k) {
		v.aggregatedSlotCommitteeIDCacheLock.Unlock()
		return
	}
	v.aggregatedSlotCommitteeIDCache.Add(k, true)
	v.aggregatedSlotCommitteeIDCacheLock.Unlock()

	var slotSig []byte
	if v.distributed {
		slotSig, err = v.attSelection(attSelectionKey{slot: slot, index: duty.ValidatorIndex})
		if err != nil {
			log.WithError(err).Error("Could not find aggregated selection proof")
			if v.emitAccountMetrics {
				ValidatorAggFailVec.WithLabelValues(fmtKey).Inc()
			}
			return
		}
	} else {
		slotSig, err = v.signSlotWithSelectionProof(ctx, pubKey, slot)
		if err != nil {
			log.WithError(err).Error("Could not sign slot")
			if v.emitAccountMetrics {
				ValidatorAggFailVec.WithLabelValues(fmtKey).Inc()
			}
			return
		}
	}

	// As specified in spec, an aggregator should wait until two thirds of the way through slot
	// to broadcast the best aggregate to the global aggregate channel.
	// https://github.com/ethereum/consensus-specs/blob/v0.9.3/specs/validator/0_beacon-chain-validator.md#broadcast-aggregate
	v.waitAggregatorDuty(ctx, slot)

	postElectra := slots.ToEpoch(slot) >= params.BeaconConfig().ElectraForkEpoch

	aggSelectionRequest := &ethpb.AggregateSelectionRequest{
		Slot:           slot,
		CommitteeIndex: duty.CommitteeIndex,
		PublicKey:      pubKey[:],
		SlotSignature:  slotSig,
	}
	var agg ethpb.AggregateAttAndProof
	if postElectra {
		res, err := v.validatorClient.SubmitAggregateSelectionProofElectra(ctx, aggSelectionRequest, duty.ValidatorIndex, uint64(len(duty.Committee)))
		if err != nil {
			v.handleSubmitAggSelectionProofError(err, slot, fmtKey)
			return
		}
		agg = res.AggregateAndProof
	} else {
		res, err := v.validatorClient.SubmitAggregateSelectionProof(ctx, aggSelectionRequest, duty.ValidatorIndex, uint64(len(duty.Committee)))
		if err != nil {
			v.handleSubmitAggSelectionProofError(err, slot, fmtKey)
			return
		}
		agg = res.AggregateAndProof
	}

	sig, err := v.aggregateAndProofSig(ctx, pubKey, agg, slot)
	if err != nil {
		log.WithError(err).Error("Could not sign aggregate and proof")
		return
	}

	if postElectra {
		msg, ok := agg.(*ethpb.AggregateAttestationAndProofElectra)
		if !ok {
			log.Errorf("Message is not %T", &ethpb.AggregateAttestationAndProofElectra{})
			if v.emitAccountMetrics {
				ValidatorAggFailVec.WithLabelValues(fmtKey).Inc()
			}
			return
		}
		_, err = v.validatorClient.SubmitSignedAggregateSelectionProofElectra(ctx, &ethpb.SignedAggregateSubmitElectraRequest{
			SignedAggregateAndProof: &ethpb.SignedAggregateAttestationAndProofElectra{
				Message:   msg,
				Signature: sig,
			},
		})
		if err != nil {
			log.WithError(err).Error("Could not submit signed aggregate and proof to beacon node")
			if v.emitAccountMetrics {
				ValidatorAggFailVec.WithLabelValues(fmtKey).Inc()
			}
			return
		}
	} else {
		msg, ok := agg.(*ethpb.AggregateAttestationAndProof)
		if !ok {
			log.Errorf("Message is not %T", &ethpb.AggregateAttestationAndProof{})
			if v.emitAccountMetrics {
				ValidatorAggFailVec.WithLabelValues(fmtKey).Inc()
			}
			return
		}
		_, err = v.validatorClient.SubmitSignedAggregateSelectionProof(ctx, &ethpb.SignedAggregateSubmitRequest{
			SignedAggregateAndProof: &ethpb.SignedAggregateAttestationAndProof{
				Message:   msg,
				Signature: sig,
			},
		})
		if err != nil {
			log.WithError(err).Error("Could not submit signed aggregate and proof to beacon node")
			if v.emitAccountMetrics {
				ValidatorAggFailVec.WithLabelValues(fmtKey).Inc()
			}
			return
		}
	}

	if err := v.saveSubmittedAtt(agg.AggregateVal().GetData(), pubKey[:], true); err != nil {
		log.WithError(err).Error("Could not add aggregator indices to logs")
		if v.emitAccountMetrics {
			ValidatorAggFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}
	if v.emitAccountMetrics {
		ValidatorAggSuccessVec.WithLabelValues(fmtKey).Inc()
	}
}

// Signs input slot with domain selection proof. This is used to create the signature for aggregator selection.
func (v *validator) signSlotWithSelectionProof(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, slot primitives.Slot) (signature []byte, err error) {
	ctx, span := trace.StartSpan(ctx, "validator.signSlotWithSelectionProof")
	defer span.End()

	domain, err := v.domainData(ctx, slots.ToEpoch(slot), params.BeaconConfig().DomainSelectionProof[:])
	if err != nil {
		return nil, err
	}

	var sig bls.Signature
	sszUint := primitives.SSZUint64(slot)
	root, err := signing.ComputeSigningRoot(&sszUint, domain.SignatureDomain)
	if err != nil {
		return nil, err
	}
	sig, err = v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     root[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          &validatorpb.SignRequest_Slot{Slot: slot},
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, err
	}

	return sig.Marshal(), nil
}

// waitAggregatorDuty waits for aggregator duty given the slot intervals.
func (v *validator) waitAggregatorDuty(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.waitAggregatorDuty")
	defer span.End()

	var delay time.Duration
	if slots.ToEpoch(slot) >= params.BeaconConfig().EPBSForkEpoch {
		delay = slots.DivideSlotBy(int64(params.BeaconConfig().IntervalsPerSlotEPBS))
	} else {
		delay = slots.DivideSlotBy(int64(params.BeaconConfig().IntervalsPerSlot))
	}
	delay *= 2

	startTime := slots.StartTime(v.genesisTime, slot)
	finalTime := startTime.Add(delay)
	wait := prysmTime.Until(finalTime)
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

// This returns the signature of validator signing over aggregate and
// proof object.
func (v *validator) aggregateAndProofSig(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, agg ethpb.AggregateAttAndProof, slot primitives.Slot) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "validator.aggregateAndProofSig")
	defer span.End()

	d, err := v.domainData(ctx, slots.ToEpoch(agg.AggregateVal().GetData().Slot), params.BeaconConfig().DomainAggregateAndProof[:])
	if err != nil {
		return nil, err
	}
	root, err := signing.ComputeSigningRoot(agg, d.SignatureDomain)
	if err != nil {
		return nil, err
	}

	signRequest := &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     root[:],
		SignatureDomain: d.SignatureDomain,
		SigningSlot:     slot,
	}
	if agg.Version() >= version.Electra {
		aggregate, ok := agg.(*ethpb.AggregateAttestationAndProofElectra)
		if !ok {
			return nil, fmt.Errorf("wrong aggregate type (expected %T, got %T)", &ethpb.AggregateAttestationAndProofElectra{}, agg)
		}
		signRequest.Object = &validatorpb.SignRequest_AggregateAttestationAndProofElectra{AggregateAttestationAndProofElectra: aggregate}
	} else {
		aggregate, ok := agg.(*ethpb.AggregateAttestationAndProof)
		if !ok {
			return nil, fmt.Errorf("wrong aggregate type (expected %T, got %T)", &ethpb.AggregateAttestationAndProof{}, agg)
		}
		signRequest.Object = &validatorpb.SignRequest_AggregateAttestationAndProof{AggregateAttestationAndProof: aggregate}
	}

	sig, err := v.km.Sign(ctx, signRequest)
	if err != nil {
		return nil, err
	}

	return sig.Marshal(), nil
}

func (v *validator) handleSubmitAggSelectionProofError(err error, slot primitives.Slot, hexPubkey string) {
	// handle grpc not found
	s, ok := status.FromError(err)
	grpcNotFound := ok && s.Code() == codes.NotFound
	// handle http not found
	jsonErr := &httputil.DefaultJsonError{}
	httpNotFound := errors.As(err, &jsonErr) && jsonErr.Code == http.StatusNotFound

	if grpcNotFound || httpNotFound {
		log.WithField("slot", slot).WithError(err).Warn("No attestations to aggregate")
	} else {
		log.WithField("slot", slot).WithError(err).Error("Could not submit aggregate selection proof to beacon node")
		if v.emitAccountMetrics {
			ValidatorAggFailVec.WithLabelValues(hexPubkey).Inc()
		}
	}
}
