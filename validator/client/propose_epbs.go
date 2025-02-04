package client

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

// SubmitHeader submits a signed execution payload header to the validator client.
func (v *validator) SubmitHeader(ctx context.Context, slot primitives.Slot, pubKey [48]byte) error {
	if params.BeaconConfig().EPBSForkEpoch > slots.ToEpoch(slot) {
		return nil
	}
	log.WithFields(logrus.Fields{
		"slot":   slot,
		"pubkey": fmt.Sprintf("%x", pubKey),
	}).Info("Submitting header")

	proposerIndex, ok := v.pubkeyToStatus[pubKey]
	if !ok {
		return fmt.Errorf("validator index not found for pubkey %v", pubKey)
	}
	header, err := v.validatorClient.GetLocalHeader(ctx, &ethpb.HeaderRequest{
		Slot:          slot,
		ProposerIndex: proposerIndex.index,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get local header")
	}

	sig, err := v.signExecutionPayloadHeader(ctx, header, pubKey)
	if err != nil {
		return errors.Wrap(err, "failed to sign execution payload header")
	}

	if _, err := v.validatorClient.SubmitSignedExecutionPayloadHeader(ctx, &enginev1.SignedExecutionPayloadHeader{
		Message:   header,
		Signature: sig,
	}); err != nil {
		return errors.Wrap(err, "failed to submit signed execution payload header")
	}

	return nil
}

// SubmitExecutionPayloadEnvelope submits a signed execution payload envelope to the validator client.
func (v *validator) SubmitExecutionPayloadEnvelope(ctx context.Context, slot primitives.Slot, pubKey [48]byte) error {
	if params.BeaconConfig().EPBSForkEpoch > slots.ToEpoch(slot) {
		return nil
	}
	log.WithFields(logrus.Fields{
		"slot":   slot,
		"pubkey": fmt.Sprintf("%x", pubKey),
	}).Info("Submitting payload envelope")

	proposerIndex, ok := v.pubkeyToStatus[pubKey]
	if !ok {
		return fmt.Errorf("validator index not found for pubkey %v", pubKey)
	}
	env, err := v.validatorClient.GetExecutionPayloadEnvelope(ctx, &ethpb.PayloadEnvelopeRequest{
		Slot:          slot,
		ProposerIndex: proposerIndex.index,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get execution payload envelope")
	}

	sig, err := v.signExecutionPayloadEnvelope(ctx, env, pubKey)
	if err != nil {
		return errors.Wrap(err, "failed to sign execution payload envelope")
	}

	if _, err := v.validatorClient.SubmitSignedExecutionPayloadEnvelope(ctx, &enginev1.SignedExecutionPayloadEnvelope{
		Message:   env,
		Signature: sig,
	}); err != nil {
		return errors.Wrap(err, "failed to submit signed execution payload envelope")
	}

	return nil
}
