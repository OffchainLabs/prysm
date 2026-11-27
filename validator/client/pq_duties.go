package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// pqSignedPreferencesSlotsKept is the number of proposal slots before the last signed one whose signed proposer
// preferences `pqSignedPreferences` keeps.
const pqSignedPreferencesSlotsKept = 1024

// errNoPQSigner is returned when signing in the PQ fork before the keys are loaded.
var errNoPQSigner = errors.New("no XMSS signer: the post-quantum keys are not loaded")

// pqSignedPreferences holds, by proposal slot, the proposer preferences signed with XMSS keys. It is safe for
// concurrent use, and its zero value is ready to use.
type pqSignedPreferences struct {
	mu     sync.Mutex
	bySlot map[primitives.Slot]*ethpb.SignedProposerPreferences
}

// performRolePQ performs `role` at `slot` for the validator whose BLS public key is `pubKey`, in the PQ fork.
func (v *validator) performRolePQ(ctx context.Context, role validatorRole, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	logger := log.WithFields(logrus.Fields{
		"pubkey": fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:])),
		"slot":   slot,
	})

	var err error
	switch role {
	case roleAttester:
		err = v.submitAttestationPQ(ctx, slot, pubKey)
	case roleAggregator:
		err = v.submitAggregateAndProofPQ(ctx, slot, pubKey)
	case roleProposer:
		err = v.proposeBlockPQ(ctx, slot, pubKey)
	case rolePTCMember:
		// The Gloas flow signs with the XMSS key in the PQ fork.
		v.SubmitPayloadAttestation(ctx, slot, pubKey)
	case roleUnknown:
		logger.Trace("No active roles, doing nothing")
	default:
		logger.WithField("role", role).Warn("Unhandled role in the PQ fork")
	}

	if err != nil {
		logger.WithError(err).WithField("role", role).Error("Could not perform duty in the PQ fork")
	}
}

// pqSigningContext returns the XMSS public key of the validator whose BLS public key is `pubKey`, the PQ part of
// the validator client, and the XMSS signer.
func (v *validator) pqSigningContext(pubKey [fieldparams.BLSPubkeyLength]byte) (
	[fieldparams.XMSSPubkeyLength]byte,
	iface.PQValidatorClient,
	error,
) {
	client, ok := v.pqClient()
	if !ok {
		return [fieldparams.XMSSPubkeyLength]byte{}, nil, errors.New("the validator client has no PQ beacon API client")
	}

	if v.pqSigner == nil {
		return [fieldparams.XMSSPubkeyLength]byte{}, nil, errNoPQSigner
	}

	xmssKey, ok := v.pqKeys.XMSSPubkey(pubKey)
	if !ok {
		return [fieldparams.XMSSPubkeyLength]byte{}, nil, fmt.Errorf("no XMSS key for %#x", pubKey)
	}

	return xmssKey, client, nil
}

// pqSigningRoot returns the signing root of `object` with the domain of type `domainType` at `epoch`.
func (v *validator) pqSigningRoot(ctx context.Context, object ssz.HashRoot, epoch primitives.Epoch, domainType [4]byte) ([32]byte, error) {
	domain, err := v.domainData(ctx, epoch, domainType[:])
	if err != nil {
		return [32]byte{}, fmt.Errorf("domain data: %w", err)
	}

	root, err := signing.ComputeSigningRoot(object, domain.SignatureDomain)
	if err != nil {
		return [32]byte{}, fmt.Errorf("signing root: %w", err)
	}

	return root, nil
}

// submitAttestationPQ signs and submits the attestation of the validator whose BLS public key is `pubKey` at `slot`.
func (v *validator) submitAttestationPQ(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) error {
	xmssKey, client, err := v.pqSigningContext(pubKey)
	if err != nil {
		return fmt.Errorf("signing context: %w", err)
	}

	v.waitUntilAttestationDueOrValidBlock(ctx, slot)

	duty, err := v.duty(pubKey)
	if err != nil {
		return fmt.Errorf("duty: %w", err)
	}

	if duty.CommitteeLength == 0 {
		return nil
	}

	data, err := v.getAttestationData(ctx, slot, duty.CommitteeIndex)
	if err != nil {
		return fmt.Errorf("attestation data: %w", err)
	}

	// The signing protection database records the target epoch of the attestation, and a PQ attestation signs the
	// payload status in its index. Refuse anything else.
	epoch := slots.ToEpoch(slot)
	if data.Slot != slot || data.Source == nil || data.Target == nil || data.Target.Epoch != epoch || data.CommitteeIndex > 1 {
		return fmt.Errorf("attestation data for slot %d, target epoch %d and index %d is not signable at slot %d",
			data.Slot, data.GetTarget().GetEpoch(), data.CommitteeIndex, slot)
	}

	root, err := v.pqSigningRoot(ctx, data, data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		return fmt.Errorf("attestation signing root: %w", err)
	}

	// A signature already uses all the CPUs, so signing in parallel only delays the first attestations.
	v.pqAttestationSigningLock.Lock()
	if err := ctx.Err(); err != nil {
		v.pqAttestationSigningLock.Unlock()
		return fmt.Errorf("wait to sign attestation: %w", err)
	}

	signingStart := time.Now()
	signature, err := v.pqSigner.SignAttestation(xmssKey, data.Source.Epoch, data.Target.Epoch, root)
	signingTime := time.Since(signingStart)
	v.pqAttestationSigningLock.Unlock()
	if err != nil {
		return fmt.Errorf("sign attestation: %w", err)
	}

	attestation := &ethpb.SingleAttestationPQ{
		CommitteeId:   duty.CommitteeIndex,
		AttesterIndex: duty.ValidatorIndex,
		Data:          data,
		Signature:     signature.Bytes(),
	}

	if err := client.SubmitAttestationsPQ(ctx, []*ethpb.SingleAttestationPQ{attestation}); err != nil {
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmt.Sprintf("%#x", pubKey)).Inc()
		}

		return fmt.Errorf("submit attestation: %w", err)
	}

	// Logged once per slot for all keys, by LogSubmissions.
	if err := v.saveSubmittedAttPQ(attestation, pubKey[:], signingTime); err != nil {
		if v.emitAccountMetrics {
			ValidatorAttestFailVec.WithLabelValues(fmt.Sprintf("%#x", pubKey)).Inc()
		}

		return fmt.Errorf("save the submitted attestation for logging: %w", err)
	}

	if v.emitAccountMetrics {
		ValidatorAttestSuccessVec.WithLabelValues(fmt.Sprintf("%#x", pubKey)).Inc()
		ValidatorAttestedSlotsGaugeVec.WithLabelValues(fmt.Sprintf("%#x", pubKey)).Set(float64(slot))
	}

	return nil
}

// submitAggregateAndProofPQ fetches, signs and submits the aggregate of the committee of the validator whose BLS
// public key is `pubKey` at `slot`. The beacon node selected the validator as an aggregator of its committee, and
// proves the aggregate of the attestations it gathered for it.
func (v *validator) submitAggregateAndProofPQ(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) error {
	xmssKey, client, err := v.pqSigningContext(pubKey)
	if err != nil {
		return fmt.Errorf("signing context: %w", err)
	}

	duty, err := v.duty(pubKey)
	if err != nil {
		return fmt.Errorf("duty: %w", err)
	}

	// The aggregators of a committee are staggered: each one broadcasts `AGGREGATOR_DELAY_BPS` after the previous one.
	position := client.AggregatorPositionPQ(slot, duty.ValidatorIndex)
	start, err := slots.StartTime(v.genesisTime, slot)
	if err != nil {
		return fmt.Errorf("slot start: %w", err)
	}

	select {
	case <-time.After(prysmTime.Until(start.Add(params.BeaconConfig().AggregateDuePQ(position)))):
	case <-ctx.Done():
		return ctx.Err()
	}

	// The attestation data of the slot is cached: it is the one the attesters of the slot signed.
	data, err := v.getAttestationData(ctx, slot, duty.CommitteeIndex)
	if err != nil {
		return fmt.Errorf("attestation data: %w", err)
	}

	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("attestation data root: %w", err)
	}

	aggregate, proof, err := client.AggregatePQ(ctx, slot, dataRoot, duty.CommitteeIndex)
	if err != nil {
		return fmt.Errorf("aggregate: %w", err)
	}

	if !proto.Equal(aggregate.Data, data) {
		return errors.New("the aggregate is not of the requested attestation data")
	}

	if committees := aggregate.CommitteeBits.BitIndices(); len(committees) != 1 || committees[0] != int(duty.CommitteeIndex) {
		return fmt.Errorf("the aggregate is of committees %v instead of committee %d", committees, duty.CommitteeIndex)
	}

	if len(proof) == 0 {
		return errors.New("the aggregate has no proof")
	}

	message := &ethpb.AggregateAttestationAndProofPQ{
		AggregatorIndex: duty.ValidatorIndex,
		Aggregate:       aggregate,
		Proof:           proof,
	}

	root, err := v.pqSigningRoot(ctx, message, slots.ToEpoch(slot), params.BeaconConfig().DomainAggregateAndProof)
	if err != nil {
		return fmt.Errorf("aggregate and proof signing root: %w", err)
	}

	signature, err := v.pqSigner.SignSlotMessage(xmssKey, signingprotection.AggregateAndProof, slot, root)
	if err != nil {
		return fmt.Errorf("sign aggregate and proof: %w", err)
	}

	signed := &ethpb.SignedAggregateAttestationAndProofPQ{Message: message, Signature: signature.Bytes()}
	if err := client.SubmitAggregatesPQ(ctx, []*ethpb.SignedAggregateAttestationAndProofPQ{signed}); err != nil {
		return fmt.Errorf("submit aggregate and proof: %w", err)
	}

	log.WithFields(logrus.Fields{
		"slot":            slot,
		"aggregatorIndex": duty.ValidatorIndex,
		"position":        position,
		"committeeIndex":  duty.CommitteeIndex,
		"attesterCount":   aggregate.AggregationBits.Count(),
		"proofSize":       len(proof),
	}).Info("Submitted new PQ aggregate and proof")

	return nil
}

// proposeBlockPQ proposes the block of `slot` of the validator whose BLS public key is `pubKey`: it reveals the VRF
// proof of the slot, signs the block the beacon node builds, and then the envelope of its payload.
func (v *validator) proposeBlockPQ(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) error {
	xmssKey, client, err := v.pqSigningContext(pubKey)
	if err != nil {
		return fmt.Errorf("signing context: %w", err)
	}

	if slot == 0 {
		return nil
	}

	duty, err := v.duty(pubKey)
	if err != nil {
		return fmt.Errorf("duty: %w", err)
	}

	tree, ok := v.pqKeys.VRF(pubKey)
	if !ok {
		return fmt.Errorf("no VRF tree: the VRF state of the validator is not loaded, check --%s", flags.VRFStateURLFlag.Name)
	}

	nodes, err := tree.Proof(slot)
	if err != nil {
		return fmt.Errorf("VRF proof: %w", err)
	}

	randaoReveal := make([]byte, 0, len(nodes)*fieldparams.RootLength)
	for _, node := range nodes {
		randaoReveal = append(randaoReveal, node[:]...)
	}

	graffiti, err := v.Graffiti(ctx, pubKey)
	if err != nil {
		log.WithError(err).Warn("Could not get graffiti, proposing without it")
		graffiti = nil
	}

	contents, err := client.ProduceBlockPQ(ctx, slot, randaoReveal, graffiti)
	if err != nil {
		return fmt.Errorf("produce block: %w", err)
	}

	block, envelope := contents.Block, contents.Envelope
	if block == nil || envelope == nil {
		return errors.New("the beacon node returned no block or no envelope")
	}

	if block.Slot != slot || block.ProposerIndex != duty.ValidatorIndex {
		return fmt.Errorf("the block of slot %d and proposer %d is not the block of slot %d and proposer %d",
			block.Slot, block.ProposerIndex, slot, duty.ValidatorIndex)
	}

	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("block root: %w", err)
	}

	if [32]byte(envelope.BeaconBlockRoot) != blockRoot {
		return fmt.Errorf("the envelope is of block %#x instead of block %#x", envelope.BeaconBlockRoot, blockRoot)
	}

	epoch := slots.ToEpoch(slot)

	root, err := v.pqSigningRoot(ctx, block, epoch, params.BeaconConfig().DomainBeaconProposer)
	if err != nil {
		return fmt.Errorf("block signing root: %w", err)
	}

	signature, err := v.pqSigner.SignSlotMessage(xmssKey, signingprotection.Block, slot, root)
	if err != nil {
		return fmt.Errorf("sign block: %w", err)
	}

	if err := client.PublishBlockPQ(ctx, &ethpb.SignedBeaconBlockPQ{Block: block, Signature: signature.Bytes()}); err != nil {
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmt.Sprintf("%#x", pubKey)).Inc()
		}

		return fmt.Errorf("publish block: %w", err)
	}

	root, err = v.pqSigningRoot(ctx, envelope, epoch, params.BeaconConfig().DomainBeaconBuilder)
	if err != nil {
		return fmt.Errorf("envelope signing root: %w", err)
	}

	signature, err = v.pqSigner.SignSlotMessage(xmssKey, signingprotection.ExecutionPayloadEnvelope, slot, root)
	if err != nil {
		return fmt.Errorf("sign envelope: %w", err)
	}

	signedEnvelope := &ethpb.SignedExecutionPayloadEnvelopePQ{Message: envelope, Signature: signature.Bytes()}
	if err := client.PublishEnvelopePQ(ctx, signedEnvelope, contents.KzgProofs, contents.Blobs); err != nil {
		return fmt.Errorf("publish envelope: %w", err)
	}

	if v.emitAccountMetrics {
		ValidatorProposeSuccessVec.WithLabelValues(fmt.Sprintf("%#x", pubKey)).Inc()
	}

	log.WithFields(logrus.Fields{
		"slot":             slot,
		"proposerIndex":    duty.ValidatorIndex,
		"blockRoot":        fmt.Sprintf("%#x", bytesutil.Trunc(blockRoot[:])),
		"attestationCount": len(block.Body.Attestations),
		"blobCount":        len(contents.Blobs),
		"graffiti":         string(bytesutil.SafeCopyBytes(graffiti)),
	}).Info("Submitted new PQ block and payload envelope")

	return nil
}

// signPayloadAttestationPQ signs `data`, the payload attestation data of the validator whose BLS public key is
// `pubKey`, at the leaf of its slot.
func (v *validator) signPayloadAttestationPQ(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, data *ethpb.PayloadAttestationData) ([]byte, error) {
	xmssKey, _, err := v.pqSigningContext(pubKey)
	if err != nil {
		return nil, fmt.Errorf("signing context: %w", err)
	}

	root, err := v.pqSigningRoot(ctx, data, slots.ToEpoch(data.Slot), params.BeaconConfig().DomainPTCAttester)
	if err != nil {
		return nil, fmt.Errorf("payload attestation signing root: %w", err)
	}

	signature, err := v.pqSigner.SignSlotMessage(xmssKey, signingprotection.PayloadAttestation, data.Slot, root)
	if err != nil {
		return nil, fmt.Errorf("sign payload attestation: %w", err)
	}

	return signature.Bytes(), nil
}

// signProposerPreferencesPQ signs `preferences`, the proposer preferences of the validator whose BLS public key is
// `pubKey`, at the leaf of its proposal slot. The leaf protection refuses to sign other preferences for the same
// proposal slot, as the spec requires, even if the dependent root changed.
//
// A leaf signs a single message. To submit the same preferences again, for instance after the beacon node refused
// them, it returns the preferences it already signed instead of signing them again.
func (v *validator) signProposerPreferencesPQ(
	ctx context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	preferences *ethpb.ProposerPreferences,
) (*ethpb.SignedProposerPreferences, error) {
	if signed, ok := v.pqSignedPreferences.get(preferences); ok {
		return signed, nil
	}

	xmssKey, _, err := v.pqSigningContext(pubKey)
	if err != nil {
		return nil, fmt.Errorf("signing context: %w", err)
	}

	epoch := slots.ToEpoch(preferences.ProposalSlot)
	root, err := v.pqSigningRoot(ctx, preferences, epoch, params.BeaconConfig().DomainProposerPreferences)
	if err != nil {
		return nil, fmt.Errorf("proposer preferences signing root: %w", err)
	}

	signature, err := v.pqSigner.SignSlotMessage(xmssKey, signingprotection.ProposerPreferences, preferences.ProposalSlot, root)
	if err != nil {
		return nil, fmt.Errorf("sign proposer preferences: %w", err)
	}

	signed := &ethpb.SignedProposerPreferences{Message: preferences, Signature: signature.Bytes()}
	v.pqSignedPreferences.set(signed)

	return signed, nil
}

// get returns the signed proposer preferences whose message is `preferences`, if any.
func (p *pqSignedPreferences) get(preferences *ethpb.ProposerPreferences) (*ethpb.SignedProposerPreferences, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	signed, ok := p.bySlot[preferences.ProposalSlot]
	if !ok || !proto.Equal(signed.Message, preferences) {
		return nil, false
	}

	return signed, true
}

// set records `signed`, and forgets the preferences of proposal slots too old to be submitted again.
func (p *pqSignedPreferences) set(signed *ethpb.SignedProposerPreferences) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.bySlot == nil {
		p.bySlot = make(map[primitives.Slot]*ethpb.SignedProposerPreferences)
	}

	slot := signed.Message.ProposalSlot
	p.bySlot[slot] = signed

	for known := range p.bySlot {
		if known+pqSignedPreferencesSlotsKept < slot {
			delete(p.bySlot, known)
		}
	}
}

// submitProposerPreferencesPQ submits `preferences`, signed with XMSS keys.
func (v *validator) submitProposerPreferencesPQ(ctx context.Context, preferences []*ethpb.SignedProposerPreferences) error {
	client, ok := v.pqClient()
	if !ok {
		return errors.New("the validator client has no PQ beacon API client")
	}

	pqPreferences := make([]*ethpb.SignedProposerPreferencesPQ, 0, len(preferences))
	for _, preference := range preferences {
		pqPreferences = append(pqPreferences, &ethpb.SignedProposerPreferencesPQ{
			Message:   preference.Message,
			Signature: preference.Signature,
		})
	}

	if err := client.SubmitProposerPreferencesPQ(ctx, pqPreferences); err != nil {
		return fmt.Errorf("submit PQ proposer preferences: %w", err)
	}

	return nil
}
