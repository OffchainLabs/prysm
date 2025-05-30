package client

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	v1alpha1 "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/validator-client"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// SubmitInclusionList submits a signed inclusion list for a given slot and public key.
// It retrieves the inclusion list, assigns the validator index, signs the list, and submits it to the beacon node RPC server.
func (v *validator) SubmitInclusionList(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	// Ensure we are past the Fulu fork epoch.
	if params.BeaconConfig().Eip7805ForkEpoch > slots.ToEpoch(slot) {
		return
	}

	// Wait until it is time to process the inclusion list duty for the slot.
	v.waitForInclusionList(ctx, slot)

	// Retrieve the inclusion list for the specified slot.
	il, err := v.validatorClient.GetInclusionList(&v1alpha1.GetInclusionListRequest{Slot: slot})
	if err != nil {
		log.WithError(err).Error("could not get inclusion list")
		return
	}

	status, found := v.pubkeyToStatus[pubKey]
	if !found {
		log.WithField("pubkey", pubKey).Error("could not find validator index for pubkey")
		return
	}
	index := status.index
	il.ValidatorIndex = index

	// Sign the inclusion list.
	sig, err := v.signInclusionList(ctx, il, pubKey)
	if err != nil {
		log.WithError(err).Error("could not sign inclusion list")
		return
	}

	// Submit the signed inclusion list to the beacon node server.
	if _, err := v.validatorClient.SubmitInclusionList(&v1alpha1.SignedInclusionList{
		Message:   il,
		Signature: sig,
	}); err != nil {
		log.WithError(err).Error("could not submit signed inclusion list")
	}

	log.WithFields(logrus.Fields{
		"slot":          slot,
		"pubkey":        fmt.Sprintf("%#x", pubKey),
		"committeeRoot": fmt.Sprintf("%#x", il.InclusionListCommitteeRoot),
		"txCount":       len(il.Transactions),
	}).Info("Submitted inclusion list")
}

func (v *validator) signInclusionList(ctx context.Context, il *v1alpha1.InclusionList, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	currentSlot := slots.CurrentSlot(v.genesisTime)
	epoch := slots.ToEpoch(currentSlot)

	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainInclusionListCommittee[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve domain data")
	}
	if domain == nil {
		return nil, errors.New("domain data is nil")
	}

	signingRoot, err := signing.ComputeSigningRoot(il, domain.SignatureDomain)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute signing root")
	}

	signReq := &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     signingRoot[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          &validatorpb.SignRequest_InclusionList{InclusionList: il},
		SigningSlot:     currentSlot,
	}

	m, err := v.Keymanager()
	if err != nil {
		return nil, errors.Wrap(err, "could not get key manager")
	}
	sig, err := m.Sign(ctx, signReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to sign inclusion list")
	}

	return sig.Marshal(), nil
}

// waitForInclusionList waits until it is time to process the inclusion list duty for the specified slot.
// It calculates the wait duration based on the slot start time and the inclusion list duty time.
func (v *validator) waitForInclusionList(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.waitUntilInclusionListDuty")
	defer span.End()

	startTime := slots.StartTime(v.genesisTime, slot)
	submissionDeadline := params.BeaconConfig().SecondsPerSlot * 2 / params.BeaconConfig().IntervalsPerSlot
	dutyTime := startTime.Add(time.Duration(submissionDeadline-1) * time.Second)

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
