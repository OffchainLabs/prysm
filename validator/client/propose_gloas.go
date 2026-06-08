package client

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// signExecutionPayloadEnvelope signs the execution payload envelope using the
// proposer's key. The envelope is signed with DomainBeaconBuilder since it is
// a builder artifact — even in the self-build case where the proposer acts as
// their own builder.
func (v *validator) signExecutionPayloadEnvelope(
	ctx context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	slot primitives.Slot,
	envelope *ethpb.ExecutionPayloadEnvelope,
) (*ethpb.SignedExecutionPayloadEnvelope, error) {
	ctx, span := trace.StartSpan(ctx, "validator.signExecutionPayloadEnvelope")
	defer span.End()

	epoch := slots.ToEpoch(slot)

	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainBeaconBuilder[:])
	if err != nil {
		return nil, errors.Wrap(err, "could not get domain data")
	}
	if domain == nil {
		return nil, errors.New("nil domain data")
	}

	signingRoot, err := signing.ComputeSigningRoot(envelope, domain.SignatureDomain)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute signing root")
	}

	sig, err := v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     signingRoot[:],
		SignatureDomain: domain.SignatureDomain,
		Object: &validatorpb.SignRequest_ExecutionPayloadEnvelope{
			ExecutionPayloadEnvelope: envelope,
		},
		SigningSlot: slot,
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not sign execution payload envelope")
	}

	return &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: sig.Marshal(),
	}, nil
}

func (v *validator) proposeSelfBuildEnvelope(
	ctx context.Context,
	slot primitives.Slot,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	blk interfaces.SignedBeaconBlock,
) error {
	if blk.Version() < version.Gloas {
		return nil
	}

	bid, err := blk.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		return err
	}
	if bid == nil || bid.Message == nil {
		return errors.New("no execution payload bid found in block body")
	}
	if bid.Message.BuilderIndex != params.BeaconConfig().BuilderIndexSelfBuild {
		// only used for self build
		return nil
	}

	envelope, err := v.validatorClient.GetExecutionPayloadEnvelope(ctx, slot)
	if err != nil {
		validatorSelfBuildEnvelopeSubmissionTotal.WithLabelValues("failed").Inc()
		return errors.Wrap(err, "failed to get execution payload envelope for self-build")
	}

	signedEnvelope, err := v.signExecutionPayloadEnvelope(ctx, pubKey, slot, envelope)
	if err != nil {
		validatorSelfBuildEnvelopeSubmissionTotal.WithLabelValues("failed").Inc()
		return errors.Wrap(err, "could not sign execution payload envelope")
	}

	if _, err := v.validatorClient.PublishExecutionPayloadEnvelope(ctx, signedEnvelope); err != nil {
		validatorSelfBuildEnvelopeSubmissionTotal.WithLabelValues("failed").Inc()
		return errors.Wrap(err, "failed to publish execution payload envelope")
	}
	validatorSelfBuildEnvelopeSubmissionTotal.WithLabelValues("success").Inc()

	return nil
}
