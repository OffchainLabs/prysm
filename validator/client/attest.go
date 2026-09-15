package client

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	otelTrace "go.opentelemetry.io/otel/trace"
)

var failedAttLocalProtectionErr = "attempted to make slashable attestation, rejected by local slashing protection"

// SubmitAttestation completes the validator client's attester responsibility at a given slot.
// It fetches the latest beacon block head along with the latest canonical beacon state
// information in order to sign the block and include information about the validator's
// participation in voting on the block.
func (v *validator) SubmitAttestation(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitAttestation")
	defer span.End()
	span.SetAttributes(trace.StringAttribute("validator", fmt.Sprintf("%#x", pubKey)))

	att, err := v.signAttestation(ctx, slot, pubKey)
	if err != nil {
		v.recordAttestationFailure(span, slot, pubKey, err)
		return
	}
	if att == nil {
		return
	}
	if err := v.submitSignedAttestation(ctx, att); err != nil {
		v.recordAttestationFailure(span, slot, pubKey, err)
		return
	}
	if err := v.recordSubmittedAttestation(span, slot, pubKey, att); err != nil {
		v.recordAttestationFailure(span, slot, pubKey, err)
	}
}

func (v *validator) signAttestation(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) (ethpb.Att, error) {
	ctx, span := trace.StartSpan(ctx, "validator.signAttestation")
	defer span.End()
	span.SetAttributes(trace.StringAttribute("validator", fmt.Sprintf("%#x", pubKey)))

	v.waitUntilAttestationDueOrValidBlock(ctx, slot)

	var b strings.Builder
	if err := b.WriteByte(byte(roleAttester)); err != nil {
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not write role byte for lock key")
	}
	_, err := b.Write(pubKey[:])
	if err != nil {
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not write pubkey bytes for lock key")
	}
	lock := async.NewMultilock(b.String())
	lock.Lock()
	defer lock.Unlock()

	duty, err := v.duty(pubKey)
	if err != nil {
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not fetch validator assignment")
	}
	if duty.CommitteeLength == 0 {
		log.WithFields(attesterLogFields(slot, pubKey)).Debug("Empty committee for validator duty, not attesting")
		return nil, nil
	}

	postElectra := slots.ToEpoch(slot) >= params.BeaconConfig().ElectraForkEpoch

	data, err := v.getAttestationData(ctx, slot, duty.CommitteeIndex)
	if err != nil {
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not request attestation to sign at slot")
	}

	sig, _, err := v.signAtt(ctx, pubKey, data, slot)
	if err != nil {
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not sign attestation")
	}

	var indexedAtt ethpb.IndexedAtt
	if postElectra {
		indexedAtt = &ethpb.IndexedAttestationElectra{
			AttestingIndices: []uint64{uint64(duty.ValidatorIndex)},
			Data:             data,
			Signature:        sig,
		}
	} else {
		indexedAtt = &ethpb.IndexedAttestation{
			AttestingIndices: []uint64{uint64(duty.ValidatorIndex)},
			Data:             data,
			Signature:        sig,
		}
	}

	_, signingRoot, err := v.domainAndSigningRoot(ctx, indexedAtt.GetData())
	if err != nil {
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "could not get domain and signing root from attestation")
	}

	if err := v.db.SlashableAttestationCheck(ctx, indexedAtt, pubKey, signingRoot, v.emitAccountMetrics, ValidatorAttestFailVec); err != nil {
		log.WithFields(
			attestationLogFields(pubKey, indexedAtt),
		).Debug("Attempted slashable attestation details")
		tracing.AnnotateError(span, err)
		return nil, errors.Wrap(err, "failed attestation slashing protection check")
	}

	if postElectra {
		return &ethpb.SingleAttestation{
			Data:          data,
			AttesterIndex: duty.ValidatorIndex,
			CommitteeId:   duty.CommitteeIndex,
			Signature:     sig,
		}, nil
	}
	aggregationBitfield := bitfield.NewBitlist(duty.CommitteeLength)
	aggregationBitfield.SetBitAt(duty.ValidatorCommitteeIndex, true)
	return &ethpb.Attestation{
		Data:            data,
		AggregationBits: aggregationBitfield,
		Signature:       sig,
	}, nil
}

// submitSignedAttestation sends a signed attestation to the beacon node.
func (v *validator) submitSignedAttestation(ctx context.Context, attestation ethpb.Att) error {
	var err error
	switch a := attestation.(type) {
	case *ethpb.SingleAttestation:
		_, err = v.validatorClient.ProposeAttestationElectra(ctx, []*ethpb.SingleAttestation{a})
	case *ethpb.Attestation:
		_, err = v.validatorClient.ProposeAttestation(ctx, []*ethpb.Attestation{a})
	default:
		return errors.Errorf("unexpected attestation type %T", a)
	}
	return errors.Wrap(err, "could not submit attestation to beacon node")
}

// recordSubmittedAttestation saves a submitted attestation for end-of-slot logging, bumps success
// metrics, and annotates the span with it.
func (v *validator) recordSubmittedAttestation(span otelTrace.Span, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, attestation ethpb.Att) error {
	if err := v.saveSubmittedAtt(attestation, pubKey[:], false); err != nil {
		return errors.Wrap(err, "could not save validator index for logging")
	}
	if v.emitAccountMetrics {
		fmtKey := fmt.Sprintf("%#x", pubKey[:])
		ValidatorAttestSuccessVec.WithLabelValues(fmtKey).Inc()
		ValidatorAttestedSlotsGaugeVec.WithLabelValues(fmtKey).Set(float64(slot))
	}

	data := attestation.GetData()
	span.SetAttributes(
		trace.Int64Attribute("slot", int64(slot)), // lint:ignore uintcast -- This conversion is OK for tracing.
		trace.StringAttribute("blockRoot", fmt.Sprintf("%#x", data.BeaconBlockRoot)),
		trace.Int64Attribute("justifiedEpoch", int64(data.Source.Epoch)),
		trace.Int64Attribute("targetEpoch", int64(data.Target.Epoch)),
	)
	if dataRoot, err := data.HashTreeRoot(); err == nil {
		span.SetAttributes(trace.StringAttribute("attestationHash", fmt.Sprintf("%#x", dataRoot[:])))
	}
	if sa, ok := attestation.(*ethpb.SingleAttestation); ok {
		span.SetAttributes(trace.Int64Attribute("attesterIndex", int64(sa.AttesterIndex)))
		span.SetAttributes(trace.Int64Attribute("committeeIndex", int64(sa.CommitteeId)))
	} else if a, ok := attestation.(*ethpb.Attestation); ok {
		span.SetAttributes(trace.StringAttribute("aggregationBitfield", fmt.Sprintf("%#x", a.AggregationBits)))
		span.SetAttributes(trace.Int64Attribute("committeeIndex", int64(data.CommitteeIndex)))
	}
	return nil
}

// recordAttestationFailure logs a failed attester duty and bumps failure metrics.
func (v *validator) recordAttestationFailure(span otelTrace.Span, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, err error) {
	log.WithError(err).WithFields(attesterLogFields(slot, pubKey)).Error("Could not submit attestation")
	if v.emitAccountMetrics {
		ValidatorAttestFailVec.WithLabelValues(fmt.Sprintf("%#x", pubKey[:])).Inc()
	}
	tracing.AnnotateError(span, err)
}

func attesterLogFields(slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) logrus.Fields {
	return logrus.Fields{
		"pubkey": fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:])),
		"slot":   slot,
	}
}

// Given the validator public key, this gets the validator assignment.
func (v *validator) duty(pubKey [fieldparams.BLSPubkeyLength]byte) (*ethpb.ValidatorDuty, error) {
	snap := v.duties.snapshot()
	if !snap.isInitialized() {
		return nil, errors.New("no duties for validators")
	}
	d, ok := snap.currentDuty(pubKey)
	if !ok {
		return nil, fmt.Errorf("pubkey %#x not in duties", bytesutil.Trunc(pubKey[:]))
	}
	return d, nil
}

// Given validator's public key, this function returns the signature of an attestation data and its signing root.
func (v *validator) signAtt(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, data *ethpb.AttestationData, slot primitives.Slot) ([]byte, [32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "validator.signAtt")
	defer span.End()

	domain, root, err := v.domainAndSigningRoot(ctx, data)
	if err != nil {
		return nil, [32]byte{}, err
	}
	sig, err := v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     root[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          &validatorpb.SignRequest_AttestationData{AttestationData: data},
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, [32]byte{}, err
	}

	return sig.Marshal(), root, nil
}

func (v *validator) domainAndSigningRoot(ctx context.Context, data *ethpb.AttestationData) (*ethpb.DomainResponse, [32]byte, error) {
	domain, err := v.domainData(ctx, data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester[:])
	if err != nil {
		return nil, [32]byte{}, err
	}
	root, err := signing.ComputeSigningRoot(data, domain.SignatureDomain)
	if err != nil {
		return nil, [32]byte{}, err
	}
	return domain, root, nil
}

// highestSlot returns the highest slot with a valid block seen by the validator
func (v *validator) highestSlot() primitives.Slot {
	v.highestValidSlotLock.Lock()
	defer v.highestValidSlotLock.Unlock()
	return v.highestValidSlot
}

// setHighestSlot sets the highest slot with a valid block seen by the validator
func (v *validator) setHighestSlot(slot primitives.Slot) {
	v.highestValidSlotLock.Lock()
	defer v.highestValidSlotLock.Unlock()
	if slot > v.highestValidSlot {
		v.highestValidSlot = slot
		v.slotFeed.Send(slot)
	}
}

// attestationDueComponent returns the slot-component basis points for the
// attestation due time.
func attestationDueComponent(slot primitives.Slot) primitives.BP {
	return params.BeaconConfig().AttestationDueBPSAtSlot(slot)
}

// waitUntilAttestationDueOrValidBlock waits until (a) or (b) whichever comes first:
//
//	(a) the validator has received a valid block that is the same slot as input slot
//	(b) the configured attestation due time has transpired (as basis points of the slot duration)
func (v *validator) waitUntilAttestationDueOrValidBlock(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.waitUntilAttestationDueOrValidBlock")
	defer span.End()

	// Don't need to wait if requested slot is the same as highest valid slot.
	if slot <= v.highestSlot() {
		return
	}

	finalTime, err := v.slotComponentDeadline(slot, attestationDueComponent(slot))
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to wait for attestation deadline")
		return
	}
	wait := prysmTime.Until(finalTime)
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()

	ch := make(chan primitives.Slot, 1)
	sub := v.slotFeed.Subscribe(ch)
	defer sub.Unsubscribe()

	for {
		select {
		case s := <-ch:
			if features.Get().AttestTimely {
				if slot <= s {
					return
				}
			}
		case <-ctx.Done():
			tracing.AnnotateError(span, ctx.Err())
			return
		case <-sub.Err():
			log.Error("Subscriber closed, exiting goroutine")
			return
		case <-t.C:
			return
		}
	}
}

func attestationLogFields(pubKey [fieldparams.BLSPubkeyLength]byte, indexedAtt ethpb.IndexedAtt) logrus.Fields {
	return logrus.Fields{
		"pubkey":         fmt.Sprintf("%#x", pubKey),
		"slot":           indexedAtt.GetData().Slot,
		"committeeIndex": indexedAtt.GetData().CommitteeIndex,
		"blockRoot":      fmt.Sprintf("%#x", indexedAtt.GetData().BeaconBlockRoot),
		"sourceEpoch":    indexedAtt.GetData().Source.Epoch,
		"sourceRoot":     fmt.Sprintf("%#x", indexedAtt.GetData().Source.Root),
		"targetEpoch":    indexedAtt.GetData().Target.Epoch,
		"targetRoot":     fmt.Sprintf("%#x", indexedAtt.GetData().Target.Root),
		"signature":      fmt.Sprintf("%#x", indexedAtt.GetSignature()),
	}
}

type signedAtt struct {
	pubKey [fieldparams.BLSPubkeyLength]byte
	att    ethpb.Att
}

// SubmitAttestations completes the attester responsibility for the given validators at a slot,
// signing per-validator as usual but submitting all attestations in a single request.
func (v *validator) SubmitAttestations(ctx context.Context, slot primitives.Slot, pubKeys [][fieldparams.BLSPubkeyLength]byte) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitAttestations")
	defer span.End()

	results := make([]signedAtt, len(pubKeys))
	var wg sync.WaitGroup
	for i, pubKey := range pubKeys {
		wg.Go(func() {
			att, err := v.signAttestation(ctx, slot, pubKey)
			if err != nil {
				v.recordAttestationFailure(span, slot, pubKey, err)
				return
			}
			if att != nil {
				results[i] = signedAtt{pubKey: pubKey, att: att}
			}
		})
	}
	wg.Wait()

	if slots.ToEpoch(slot) >= params.BeaconConfig().ElectraForkEpoch {
		var (
			attestations []*ethpb.SingleAttestation
			submitted    []signedAtt
		)
		for _, result := range results {
			if att, ok := result.att.(*ethpb.SingleAttestation); ok {
				attestations = append(attestations, att)
				submitted = append(submitted, result)
			}
		}

		if len(attestations) == 0 {
			return
		}
		_, err := v.validatorClient.ProposeAttestationElectra(ctx, attestations)
		v.recordSubmittedAttestations(span, slot, submitted, err)
		return
	}

	var (
		attestations []*ethpb.Attestation
		submitted    []signedAtt
	)
	for _, result := range results {
		if att, ok := result.att.(*ethpb.Attestation); ok {
			attestations = append(attestations, att)
			submitted = append(submitted, result)
		}
	}
	if len(attestations) == 0 {
		return
	}
	_, err := v.validatorClient.ProposeAttestation(ctx, attestations)
	v.recordSubmittedAttestations(span, slot, submitted, err)
}

// recordSubmittedAttestations applies a batch submission result to per-validator logging and metrics.
func (v *validator) recordSubmittedAttestations(span otelTrace.Span, slot primitives.Slot, results []signedAtt, err error) {
	if err == nil {
		for _, res := range results {
			if res.att != nil {
				if recErr := v.recordSubmittedAttestation(span, slot, res.pubKey, res.att); recErr != nil {
					v.recordAttestationFailure(span, slot, res.pubKey, recErr)
				}
			}
		}
		return
	}

	var indexedErr *server.IndexedErrorContainer
	if errors.As(err, &indexedErr) && len(indexedErr.Failures) > 0 {
		failedIndices := make(map[int]string, len(indexedErr.Failures))
		for _, f := range indexedErr.Failures {
			failedIndices[f.Index] = f.Message
		}

		for i, res := range results {
			if res.att == nil {
				continue
			}
			if failMsg, failed := failedIndices[i]; failed {
				v.recordAttestationFailure(span, slot, res.pubKey, errors.New(failMsg))
			} else {
				if recErr := v.recordSubmittedAttestation(span, slot, res.pubKey, res.att); recErr != nil {
					v.recordAttestationFailure(span, slot, res.pubKey, recErr)
				}
			}
		}
		return
	}

	for _, res := range results {
		if res.att != nil {
			v.recordAttestationFailure(span, slot, res.pubKey, err)
		}
	}
}
