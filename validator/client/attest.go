package client

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/async"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	"github.com/sirupsen/logrus"
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

	v.waitAttesterDuty(ctx, slot)

	var b strings.Builder
	if err := b.WriteByte(byte(iface.RoleAttester)); err != nil {
		log.WithError(err).Error("Could not write role byte for lock key")
		tracing.AnnotateError(span, err)
		return
	}
	_, err := b.Write(pubKey[:])
	if err != nil {
		log.WithError(err).Error("Could not write pubkey bytes for lock key")
		tracing.AnnotateError(span, err)
		return
	}
	lock := async.NewMultilock(b.String())
	lock.Lock()
	defer lock.Unlock()

	fmtKey := fmt.Sprintf("%#x", pubKey[:])
	log := log.WithField("pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:]))).WithField("slot", slot)
	duty, err := v.duty(pubKey)
	if err != nil {
		log.WithError(err).Error("Could not fetch validator assignment")
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}
	if len(duty.Committee) == 0 {
		log.Debug("Empty committee for validator duty, not attesting")
		return
	}

	req := &ethpb.AttestationDataRequest{
		Slot:           slot,
		CommitteeIndex: duty.CommitteeIndex,
	}
	data, err := v.validatorClient.AttestationData(ctx, req)
	if err != nil {
		log.WithError(err).Error("Could not request attestation to sign at slot")
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}

	sig, _, err := v.signAtt(ctx, pubKey, data, slot)
	if err != nil {
		log.WithError(err).Error("Could not sign attestation")
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}

	postElectra := slots.ToEpoch(slot) >= params.BeaconConfig().ElectraForkEpoch

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
		log.WithError(err).Error("Could not get domain and signing root from attestation")
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}

	var indexInCommittee uint64
	var found bool
	for i, vID := range duty.Committee {
		if vID == duty.ValidatorIndex {
			indexInCommittee = uint64(i)
			found = true
			break
		}
	}
	if !found {
		log.Errorf("Validator ID %d not found in committee of %v", duty.ValidatorIndex, duty.Committee)
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	// TODO: Extend to Electra
	phase0Att, ok := indexedAtt.(*ethpb.IndexedAttestation)
	if ok {
		// Send the attestation to the beacon node.
		if err := v.db.SlashableAttestationCheck(ctx, phase0Att, pubKey, signingRoot, v.emitAccountMetrics, ValidatorAttestFailVec); err != nil {
			log.WithError(err).Error("Failed attestation slashing protection check")
			log.WithFields(
				attestationLogFields(pubKey, indexedAtt),
			).Debug("Attempted slashable attestation details")
			tracing.AnnotateError(span, err)
			return
		}
	}

	aggregationBitfield := bitfield.NewBitlist(uint64(len(duty.Committee)))
	aggregationBitfield.SetBitAt(indexInCommittee, true)
	committeeBits := primitives.NewAttestationCommitteeBits()

	var attResp *ethpb.AttestResponse
	if postElectra {
		attestation := &ethpb.AttestationElectra{
			Data:            data,
			AggregationBits: aggregationBitfield,
			CommitteeBits:   committeeBits,
			Signature:       sig,
		}
		attestation.CommitteeBits.SetBitAt(uint64(req.CommitteeIndex), true)
		attResp, err = v.validatorClient.ProposeAttestationElectra(ctx, attestation)
	} else {
		attestation := &ethpb.Attestation{
			Data:            data,
			AggregationBits: aggregationBitfield,
			Signature:       sig,
		}
		attResp, err = v.validatorClient.ProposeAttestation(ctx, attestation)
	}
	if err != nil {
		log.WithError(err).Error("Could not submit attestation to beacon node")
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}

	if err := v.saveSubmittedAtt(data, pubKey[:], false); err != nil {
		log.WithError(err).Error("Could not save validator index for logging")
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmtKey).Inc()
		}
		tracing.AnnotateError(span, err)
		return
	}

	span.SetAttributes(
		trace.Int64Attribute("slot", int64(slot)), // lint:ignore uintcast -- This conversion is OK for tracing.
		trace.StringAttribute("attestationHash", fmt.Sprintf("%#x", attResp.AttestationDataRoot)),
		trace.StringAttribute("blockRoot", fmt.Sprintf("%#x", data.BeaconBlockRoot)),
		trace.Int64Attribute("justifiedEpoch", int64(data.Source.Epoch)),
		trace.Int64Attribute("targetEpoch", int64(data.Target.Epoch)),
		trace.StringAttribute("aggregationBitfield", fmt.Sprintf("%#x", aggregationBitfield)),
	)
	if postElectra {
		span.SetAttributes(trace.StringAttribute("committeeBitfield", fmt.Sprintf("%#x", committeeBits)))
	} else {
		span.SetAttributes(trace.Int64Attribute("committeeIndex", int64(data.CommitteeIndex)))
	}

	if v.emitAccountMetrics {
		ValidatorAttestSuccessVec.WithLabelValues(fmtKey).Inc()
		ValidatorAttestedSlotsGaugeVec.WithLabelValues(fmtKey).Set(float64(slot))
	}
}

// Given the validator public key, this gets the validator assignment.
func (v *validator) duty(pubKey [fieldparams.BLSPubkeyLength]byte) (*ethpb.DutiesResponse_Duty, error) {
	v.dutiesLock.RLock()
	defer v.dutiesLock.RUnlock()
	if v.duties == nil {
		return nil, errors.New("no duties for validators")
	}

	for _, duty := range v.duties.CurrentEpochDuties {
		if bytes.Equal(pubKey[:], duty.PublicKey) {
			return duty, nil
		}
	}

	return nil, fmt.Errorf("pubkey %#x not in duties", bytesutil.Trunc(pubKey[:]))
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

// waitAttesterDuty waits for attestor duty given the slot intervals and if current slot block has arrived.
func (v *validator) waitAttesterDuty(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.waitAttesterDuty")
	defer span.End()

	// Don't need to wait if requested slot is the same as highest valid slot.
	if slot <= v.highestSlot() {
		return
	}

	var delay time.Duration
	if slots.ToEpoch(slot) >= params.BeaconConfig().EPBSForkEpoch {
		delay = slots.DivideSlotBy(int64(params.BeaconConfig().IntervalsPerSlotEPBS))
	} else {
		delay = slots.DivideSlotBy(int64(params.BeaconConfig().IntervalsPerSlot))
	}

	startTime := slots.StartTime(v.genesisTime, slot)
	finalTime := startTime.Add(delay)
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
