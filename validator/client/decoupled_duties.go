package client

import (
	"context"
	"errors"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/decoupled"
	"github.com/sirupsen/logrus"
)

// Spec: the AC vote goes out at Δ and the FG action at 2Δ, the slot being 4Δ.
const (
	availableVoteDueBPS       primitives.BP = 2500
	finalityAttestationDueBPS primitives.BP = 5000
)

// Spec: goldfish_vote. The head is reserved for the slot in the database before signing, so a retry or a restart cannot sign a second distinct vote.
func (v *validator) SubmitAvailableAttestation(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitAvailableAttestation")
	defer span.End()
	v.waitUntilSlotComponent(ctx, slot, availableVoteDueBPS)
	data, err := v.validatorClient.AvailableAttestationData(ctx, slot)
	if err != nil {
		log.WithField("slot", slot).WithError(err).Debug("No available attestation to sign")
		return
	}
	if err := v.db.ReserveAvailableVote(ctx, pubKey, slot, bytesutil.ToBytes32(data.BeaconBlockRoot)); err != nil {
		log.WithField("slot", slot).WithError(err).Error("Refusing to sign available attestation")
		return
	}
	req := &validatorpb.SignRequest{Object: &validatorpb.SignRequest_AvailableAttestationData{AvailableAttestationData: data}, SigningSlot: slot}
	sig, idx, err := v.signDecoupled(ctx, pubKey, slots.ToEpoch(slot), params.BeaconConfig().DomainAvailableAttestation, data, req)
	if err != nil {
		log.WithField("slot", slot).WithError(err).Error("Could not sign available attestation")
		return
	}
	msg := &ethpb.AvailableAttestationMessage{ValidatorIndex: idx, Data: data, Signature: sig}
	if _, err := v.validatorClient.SubmitAvailableAttestation(ctx, msg); err != nil {
		log.WithField("slot", slot).WithError(err).Error("Could not submit available attestation")
		return
	}
	log.WithFields(logrus.Fields{"slot": slot, "head": bytesutil.Trunc(data.BeaconBlockRoot), "validatorIndex": idx}).Debug("Submitted available attestation")
}

var errFinalityPending = errors.New("a finality attestation is still pending submission")

// Spec: attest, the FG action. A row whose submission was never confirmed goes out first, before any source is asked for, across duties and restarts, since Λ already names it and a fresh decision would only suppress it.
func (v *validator) SubmitFinalityAttestation(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitFinalityAttestation")
	defer span.End()
	v.waitUntilSlotComponent(ctx, slot, finalityAttestationDueBPS)
	if row, err := v.pendingFinalityAttestation(ctx, pubKey); err != nil {
		log.WithField("slot", slot).WithError(err).Error("Refusing to sign finality attestation")
		return
	} else if row != nil && !v.submitFinalityAttestation(ctx, slot, pubKey, row, true) {
		return
	}
	data, err := v.validatorClient.FinalityAttestationData(ctx, slot)
	if err != nil {
		log.WithField("slot", slot).WithError(err).Debug("No finality source to attest to")
		return
	}
	row, err := v.decideFinalityAttestation(ctx, pubKey, data)
	if errors.Is(err, errFinalityPending) {
		log.WithField("slot", slot).Debug("Finality attestation still pending, not deciding another")
		return
	}
	if err != nil {
		log.WithField("slot", slot).WithError(err).Error("Refusing to sign finality attestation")
		return
	}
	if row == nil {
		log.WithField("slot", slot).Debug("Nothing to attest at this height")
		return
	}
	v.submitFinalityAttestation(ctx, slot, pubKey, row, false)
}

func (v *validator) submitFinalityAttestation(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, row *ethpb.AttestationDataDecoupled, resent bool) bool {
	if err := v.sendFinalityAttestation(ctx, slot, pubKey, row); err != nil {
		log.WithField("slot", slot).WithError(err).Error("Could not submit finality attestation, it is kept for the next duty")
		return false
	}
	if err := v.db.MarkFinalitySubmitted(ctx, pubKey, row); err != nil {
		log.WithField("slot", slot).WithError(err).Error("Could not mark finality attestation submitted")
		return false
	}
	log.WithFields(logrus.Fields{"slot": slot, "height": row.Height, "timeout": row.Timeout, "finalityHeight": row.FinalityHeight, "resent": resent}).Debug("Submitted finality attestation")
	return true
}

func (v *validator) pendingFinalityAttestation(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) (*ethpb.AttestationDataDecoupled, error) {
	v.finalitySigningLock.Lock()
	defer v.finalitySigningLock.Unlock()
	history, pending, _, err := v.db.FinalityHistory(ctx, pubKey)
	if err != nil || !pending || len(history) == 0 {
		return nil, err
	}
	return history[len(history)-1], nil
}

// Λ is read, the decision made and written back under one lock, so overlapping duties for a key never reserve conflicting rows, and nothing is decided behind a row still pending.
func (v *validator) decideFinalityAttestation(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, data *ethpb.FinalityAttestationData) (*ethpb.AttestationDataDecoupled, error) {
	v.finalitySigningLock.Lock()
	defer v.finalitySigningLock.Unlock()
	history, pending, known, err := v.db.FinalityHistory(ctx, pubKey)
	if err != nil {
		return nil, err
	}
	if pending && len(history) > 0 {
		return nil, errFinalityPending
	}
	if !known {
		if err := v.db.MarkFinalityKey(ctx, pubKey); err != nil {
			return nil, err
		}
	}
	record := decoupled.NewRecord()
	for _, a := range history {
		record.Restore(decoupled.Attestation{
			Height:         a.Height,
			Target:         bytesutil.ToBytes32(a.Target),
			Timeout:        a.Timeout,
			FinalityHeight: a.FinalityHeight,
			FinalityTarget: bytesutil.ToBytes32(a.FinalityTarget),
		})
	}
	src := decoupled.Source{Height: data.Height, Entry: bytesutil.ToBytes32(data.Entry)}
	head := decoupled.Head{JustifiedHeight: data.JustifiedHeight, Justified: bytesutil.ToBytes32(data.Justified), FinalizedHeight: data.FinalizedHeight}
	att, ok := record.Attest(src, head)
	if !ok {
		return nil, nil
	}
	row := &ethpb.AttestationDataDecoupled{
		Height:         att.Height,
		Target:         att.Target[:],
		Timeout:        att.Timeout,
		FinalityHeight: att.FinalityHeight,
		FinalityTarget: att.FinalityTarget[:],
	}
	if err := v.db.SaveFinalityAttestation(ctx, pubKey, row); err != nil {
		return nil, err
	}
	return row, nil
}

func (v *validator) sendFinalityAttestation(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, row *ethpb.AttestationDataDecoupled) error {
	req := &validatorpb.SignRequest{Object: &validatorpb.SignRequest_AttestationDataDecoupled{AttestationDataDecoupled: row}, SigningSlot: slot}
	sig, idx, err := v.signDecoupled(ctx, pubKey, params.BeaconConfig().DecoupledForkEpoch, params.BeaconConfig().DomainFinalityAttestation, row, req)
	if err != nil {
		return err
	}
	_, err = v.validatorClient.SubmitFinalityAttestation(ctx, &ethpb.FinalityAttestationMessage{ValidatorIndex: idx, Data: row, Signature: sig})
	return err
}

// The FG domain is asked at the fork epoch, which pins it to the Decoupled fork version.
func (v *validator) signDecoupled(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, epoch primitives.Epoch, domain [4]byte, obj ssz.HashRoot, req *validatorpb.SignRequest) ([]byte, primitives.ValidatorIndex, error) {
	d, err := v.domainData(ctx, epoch, domain[:])
	if err != nil {
		return nil, 0, err
	}
	root, err := signing.ComputeSigningRoot(obj, d.SignatureDomain)
	if err != nil {
		return nil, 0, err
	}
	req.PublicKey, req.SigningRoot, req.SignatureDomain = pubKey[:], root[:], d.SignatureDomain
	sig, err := v.km.Sign(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	duty, err := v.duty(pubKey)
	if err != nil {
		return nil, 0, err
	}
	return sig.Marshal(), duty.ValidatorIndex, nil
}
