package client

import (
	"context"
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Result labels for validatorInclusionListSubmissionTotal.
const (
	inclusionListSuccess      = "success"
	inclusionListFailed       = "failed"
	inclusionListSkippedEmpty = "skipped_empty"
	inclusionListSkippedLate  = "skipped_late"
)

// SubmitInclusionList builds, signs and publishes the inclusion list of an inclusion
// list committee member at the start of slot, before the inclusion list due point.
func (v *validator) SubmitInclusionList(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitInclusionList")
	defer span.End()
	span.SetAttributes(trace.StringAttribute("validator", fmt.Sprintf("%#x", pubKey)))

	if slots.ToEpoch(slot) < params.BeaconConfig().HezeForkEpoch {
		return
	}

	fail := func(err error, msg string) {
		validatorInclusionListSubmissionTotal.WithLabelValues(inclusionListFailed).Inc()
		tracing.AnnotateError(span, err)
		log.WithField("slot", slot).WithError(err).Error(msg)
	}

	duty, err := v.duty(pubKey)
	if err != nil {
		fail(err, "Could not fetch validator assignment")
		return
	}
	if !slices.Contains(duty.InclusionListSlots, slot) {
		fail(errors.New("no inclusion list duty for slot"), "Could not find inclusion list duty")
		return
	}
	// The inclusion list duties share the attester dependent root of the current epoch.
	dependentRoot := v.duties.prevDependentRoot()
	if len(dependentRoot) != fieldparams.RootLength {
		fail(errors.New("no dependent root for the current epoch"), "Could not find inclusion list dependent root")
		return
	}

	txs, err := v.validatorClient.ProduceInclusionList(ctx, slot)
	if err != nil {
		fail(err, "Could not produce inclusion list")
		return
	}
	if len(txs) == 0 {
		// Gossip ignores an inclusion list with no transactions.
		validatorInclusionListSubmissionTotal.WithLabelValues(inclusionListSkippedEmpty).Inc()
		log.WithField("slot", slot).Debug("Skipping inclusion list: no transactions")
		return
	}
	txList, err := enginev1.NewProgressiveTransactionList(txs)
	if err != nil {
		fail(err, "Could not build inclusion list transactions")
		return
	}

	il := &ethpb.InclusionList{
		Slot:           slot,
		ValidatorIndex: duty.ValidatorIndex,
		DependentRoot:  dependentRoot,
		Transactions:   txList,
	}
	sig, err := v.signInclusionList(ctx, il, pubKey)
	if err != nil {
		fail(err, "Could not sign inclusion list")
		return
	}

	if !v.beforeSlotComponent(slot, params.BeaconConfig().InclusionListDueBPS) {
		validatorInclusionListSubmissionTotal.WithLabelValues(inclusionListSkippedLate).Inc()
		log.WithField("slot", slot).Warn("Skipping inclusion list: past the inclusion list due point")
		return
	}

	if err := v.validatorClient.SubmitInclusionList(ctx, &ethpb.SignedInclusionList{Message: il, Signature: sig}); err != nil {
		fail(err, "Could not submit inclusion list")
		return
	}
	validatorInclusionListSubmissionTotal.WithLabelValues(inclusionListSuccess).Inc()

	var txBytes int
	for _, tx := range txs {
		txBytes += len(tx)
	}
	sinceSlotStart, err := v.sinceSlotStartTime(slot)
	if err != nil {
		log.WithError(err).Error("Failed to determine slot start time")
	}
	log.WithFields(logrus.Fields{
		"slot":               slot,
		"pubkey":             fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:])),
		"validatorIndex":     duty.ValidatorIndex,
		"txCount":            len(txs),
		"txBytes":            txBytes,
		"timeSinceSlotStart": sinceSlotStart,
	}).Info("Submitted inclusion list")
}

// signInclusionList signs il with DOMAIN_INCLUSION_LIST_COMMITTEE at the fork version of the slot epoch.
func (v *validator) signInclusionList(ctx context.Context, il *ethpb.InclusionList, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	gvr, err := v.db.GenesisValidatorsRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get genesis validators root")
	}
	fork, err := params.Fork(slots.ToEpoch(il.Slot))
	if err != nil {
		return nil, errors.Wrap(err, "could not get fork")
	}
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainInclusionListCommittee, fork.CurrentVersion, gvr)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute domain")
	}
	root, err := signing.ComputeSigningRoot(il, domain)
	if err != nil {
		return nil, errors.Wrap(err, signingRootErr)
	}
	sig, err := v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     root[:],
		SignatureDomain: domain,
		Object:          &validatorpb.SignRequest_InclusionList{InclusionList: il},
		SigningSlot:     il.Slot,
	})
	if err != nil {
		return nil, err
	}
	return sig.Marshal(), nil
}
