package sync

import (
	"context"
	"fmt"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/operation"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// validateAggregateAndProof verifies the aggregated signature and the selection proof is valid before forwarding to the
// network and downstream services.
func (s *Service) validateAggregateAndProof(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	receivedTime := prysmTime.Now()
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateAggregateAndProof")
	defer span.End()

	// To process the following it requires the recent blocks to be present in the database, so we'll skip
	// validating or processing aggregated attestations until fully synced.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	raw, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}
	m, ok := raw.(ethpb.SignedAggregateAttAndProof)
	if !ok {
		return pubsub.ValidationReject, errors.Errorf("invalid message type: %T", raw)
	}
	if m.AggregateAttestationAndProof() == nil {
		return pubsub.ValidationReject, errNilMessage
	}

	aggregate := m.AggregateAttestationAndProof().AggregateVal()
	data := aggregate.GetData()

	if err := helpers.ValidateNilAttestation(aggregate); err != nil {
		return pubsub.ValidationReject, err
	}
	// Do not process slot 0 aggregates.
	if data.Slot == 0 {
		return pubsub.ValidationIgnore, nil
	}

	// Broadcast the aggregated attestation on a feed to notify other services in the beacon node
	// of a received aggregated attestation.
	// TODO: this will be extended to Electra in a later PR
	if m.Version() == version.Phase0 {
		phase0Att, ok := m.(*ethpb.SignedAggregateAttestationAndProof)
		if ok {
			s.cfg.attestationNotifier.OperationFeed().Send(&feed.Event{
				Type: operation.AggregatedAttReceived,
				Data: &operation.AggregatedAttReceivedData{
					Attestation: phase0Att.Message,
				},
			})
		}
	}

	if err := helpers.ValidateSlotTargetEpoch(data); err != nil {
		return pubsub.ValidationReject, err
	}

	// Attestation's slot is within ATTESTATION_PROPAGATION_SLOT_RANGE and early attestation
	// processing tolerance.
	if err := helpers.ValidateAttestationTime(
		data.Slot,
		s.cfg.clock.GenesisTime(),
		earlyAttestationProcessingTolerance,
	); err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}

	// Verify this is the first aggregate received from the aggregator with index and slot.
	if s.hasSeenAggregatorIndexEpoch(data.Target.Epoch, m.AggregateAttestationAndProof().GetAggregatorIndex()) {
		return pubsub.ValidationIgnore, nil
	}
	// Check that the block being voted on isn't invalid.
	if s.hasBadBlock(bytesutil.ToBytes32(data.BeaconBlockRoot)) ||
		s.hasBadBlock(bytesutil.ToBytes32(data.Target.Root)) ||
		s.hasBadBlock(bytesutil.ToBytes32(data.Source.Root)) {
		attBadBlockCount.Inc()
		return pubsub.ValidationReject, errors.New("bad block referenced in attestation data")
	}

	// Verify aggregate attestation has not already been seen via aggregate gossip, within a block, or through the creation locally.
	seen, err := s.cfg.attPool.HasAggregatedAttestation(aggregate)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}
	if seen {
		return pubsub.ValidationIgnore, nil
	}
	if !s.validateBlockInAttestation(ctx, m) {
		return pubsub.ValidationIgnore, nil
	}

	validationRes, err := s.validateAggregatedAtt(ctx, m)
	if validationRes != pubsub.ValidationAccept {
		return validationRes, err
	}

	s.setAggregatorIndexEpochSeen(data.Target.Epoch, m.AggregateAttestationAndProof().GetAggregatorIndex())

	msg.ValidatorData = m

	aggregateAttestationVerificationGossipSummary.Observe(float64(prysmTime.Since(receivedTime).Milliseconds()))

	return pubsub.ValidationAccept, nil
}

func (s *Service) validateAggregatedAtt(ctx context.Context, signed ethpb.SignedAggregateAttAndProof) (pubsub.ValidationResult, error) {
	ctx, span := trace.StartSpan(ctx, "sync.validateAggregatedAtt")
	defer span.End()

	aggregateAndProof := signed.AggregateAttestationAndProof()
	aggregatorIndex := aggregateAndProof.GetAggregatorIndex()
	aggregate := aggregateAndProof.AggregateVal()
	data := aggregate.GetData()

	// Verify attestation target root is consistent with the head root.
	// This verification is not in the spec, however we guard against it as it opens us up
	// to weird edge cases during verification. The attestation technically could be used to add value to a block,
	// but it's invalid in the spirit of the protocol. Here we choose safety over profit.
	if err := s.cfg.chain.VerifyLmdFfgConsistency(ctx, aggregate); err != nil {
		tracing.AnnotateError(span, err)
		attBadLmdConsistencyCount.Inc()
		return pubsub.ValidationReject, err
	}

	// Verify current finalized checkpoint is an ancestor of the block defined by the attestation's beacon block root.
	if !s.cfg.chain.InForkchoice(bytesutil.ToBytes32(data.BeaconBlockRoot)) {
		tracing.AnnotateError(span, blockchain.ErrNotDescendantOfFinalized)
		return pubsub.ValidationIgnore, blockchain.ErrNotDescendantOfFinalized
	}

	bs, err := s.cfg.chain.AttestationTargetState(ctx, data.Target)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}

	committeeIndex, _, result, err := s.validateCommitteeIndexAndCount(ctx, aggregate, bs)
	if result != pubsub.ValidationAccept {
		wrappedErr := errors.Wrapf(err, "could not validate committee index")
		tracing.AnnotateError(span, wrappedErr)
		return result, err
	}

	committee, result, err := s.validateBitLength(ctx, bs, aggregate.GetData().Slot, committeeIndex, aggregate.GetAggregationBits())
	if result != pubsub.ValidationAccept {
		return result, err
	}

	// Verify validator index is within the beacon committee.
	result, err = s.validateIndexInCommittee(ctx, aggregate, aggregatorIndex, committee)
	if result != pubsub.ValidationAccept {
		wrappedErr := errors.Wrapf(err, "could not validate index in committee")
		tracing.AnnotateError(span, wrappedErr)
		return result, wrappedErr
	}

	// Verify selection proof reflects to the right validator.
	selectionSigSet, err := validateSelectionIndex(
		ctx,
		bs,
		data.Slot,
		committee,
		aggregatorIndex,
		aggregateAndProof.GetSelectionProof(),
	)
	if err != nil {
		wrappedErr := errors.Wrapf(err, "could not validate selection for validator %d", aggregateAndProof.GetAggregatorIndex())
		tracing.AnnotateError(span, wrappedErr)
		attBadSelectionProofCount.Inc()
		return pubsub.ValidationReject, wrappedErr
	}

	// Verify selection signature, aggregator signature and attestation signature are valid.
	// We use batch verify here to save compute.
	aggregatorSigSet, err := aggSigSet(bs, signed)
	if err != nil {
		wrappedErr := errors.Wrapf(err, "could not get aggregator sig set %d", aggregatorIndex)
		tracing.AnnotateError(span, wrappedErr)
		return pubsub.ValidationIgnore, wrappedErr
	}
	attSigSet, err := blocks.AttestationSignatureBatch(ctx, bs, []ethpb.Att{aggregate})
	if err != nil {
		wrappedErr := errors.Wrapf(err, "could not verify aggregator signature %d", aggregatorIndex)
		tracing.AnnotateError(span, wrappedErr)
		return pubsub.ValidationIgnore, wrappedErr
	}
	set := bls.NewSet()
	set.Join(selectionSigSet).Join(aggregatorSigSet).Join(attSigSet)

	return s.validateWithBatchVerifier(ctx, "aggregate", set)
}

func (s *Service) validateBlockInAttestation(ctx context.Context, satt ethpb.SignedAggregateAttAndProof) bool {
	// Verify the block being voted and the processed state is in beaconDB. The block should have passed validation if it's in the beaconDB.
	blockRoot := bytesutil.ToBytes32(satt.AggregateAttestationAndProof().AggregateVal().GetData().BeaconBlockRoot)
	if !s.hasBlockAndState(ctx, blockRoot) {
		// A node doesn't have the block, it'll request from peer while saving the pending attestation to a queue.
		s.savePendingAtt(satt)
		return false
	}
	return true
}

// Returns true if the node has received aggregate for the aggregator with index and target epoch.
func (s *Service) hasSeenAggregatorIndexEpoch(epoch primitives.Epoch, aggregatorIndex primitives.ValidatorIndex) bool {
	s.seenAggregatedAttestationLock.RLock()
	defer s.seenAggregatedAttestationLock.RUnlock()
	b := append(bytesutil.Bytes32(uint64(epoch)), bytesutil.Bytes32(uint64(aggregatorIndex))...)
	_, seen := s.seenAggregatedAttestationCache.Get(string(b))
	return seen
}

// Set aggregate's aggregator index target epoch as seen.
func (s *Service) setAggregatorIndexEpochSeen(epoch primitives.Epoch, aggregatorIndex primitives.ValidatorIndex) {
	s.seenAggregatedAttestationLock.Lock()
	defer s.seenAggregatedAttestationLock.Unlock()
	b := append(bytesutil.Bytes32(uint64(epoch)), bytesutil.Bytes32(uint64(aggregatorIndex))...)
	s.seenAggregatedAttestationCache.Add(string(b), true)
}

// This validates the bitfield is correct and aggregator's index in state is within the beacon committee.
// It implements the following checks from the consensus spec:
//   - [REJECT] The committee index is within the expected range -- i.e. `aggregate.data.index < get_committee_count_per_slot(state, aggregate.data.target.epoch)`.
//   - [REJECT] The number of aggregation bits matches the committee size --
//     i.e. len(aggregate.aggregation_bits) == len(get_beacon_committee(state, aggregate.data.slot, aggregate.data.index)).
//   - [REJECT] The aggregate attestation has participants -- that is, len(get_attesting_indices(state, aggregate.data, aggregate.aggregation_bits)) >= 1.
//   - [REJECT] The aggregator's validator index is within the committee --
//     i.e. `aggregate_and_proof.aggregator_index in get_beacon_committee(state, aggregate.data.slot, aggregate.data.index)`.
func (s *Service) validateIndexInCommittee(ctx context.Context, a ethpb.Att, validatorIndex primitives.ValidatorIndex, committee []primitives.ValidatorIndex) (pubsub.ValidationResult, error) {
	_, span := trace.StartSpan(ctx, "sync.validateIndexInCommittee")
	defer span.End()

	if a.GetAggregationBits().Count() == 0 {
		return pubsub.ValidationReject, errors.New("no attesting indices")
	}

	var withinCommittee bool
	for _, i := range committee {
		if validatorIndex == i {
			withinCommittee = true
			break
		}
	}
	if !withinCommittee {
		return pubsub.ValidationReject, fmt.Errorf("validator index %d is not within the committee: %v",
			validatorIndex, committee)
	}
	return pubsub.ValidationAccept, nil
}

// This validates selection proof by validating it's from the correct validator index of the slot.
// It does not verify the selection proof, it returns the signature set of selection proof which can be used for batch verify.
func validateSelectionIndex(
	ctx context.Context,
	bs state.ReadOnlyBeaconState,
	slot primitives.Slot,
	committee []primitives.ValidatorIndex,
	validatorIndex primitives.ValidatorIndex,
	proof []byte,
) (*bls.SignatureBatch, error) {
	_, span := trace.StartSpan(ctx, "sync.validateSelectionIndex")
	defer span.End()

	aggregator, err := helpers.IsAggregator(uint64(len(committee)), proof)
	if err != nil {
		return nil, err
	}
	if !aggregator {
		return nil, fmt.Errorf("validator is not an aggregator for slot %d", slot)
	}

	domain := params.BeaconConfig().DomainSelectionProof
	epoch := slots.ToEpoch(slot)

	v, err := bs.ValidatorAtIndexReadOnly(validatorIndex)
	if err != nil {
		return nil, err
	}
	pk := v.PublicKey()
	publicKey, err := bls.PublicKeyFromBytes(pk[:])
	if err != nil {
		return nil, err
	}

	d, err := signing.Domain(bs.Fork(), epoch, domain, bs.GenesisValidatorsRoot())
	if err != nil {
		return nil, err
	}
	sszUint := primitives.SSZUint64(slot)
	root, err := signing.ComputeSigningRoot(&sszUint, d)
	if err != nil {
		return nil, err
	}
	return &bls.SignatureBatch{
		Signatures:   [][]byte{proof},
		PublicKeys:   []bls.PublicKey{publicKey},
		Messages:     [][32]byte{root},
		Descriptions: []string{signing.SelectionProof},
	}, nil
}

// This returns aggregator signature set which can be used to batch verify.
func aggSigSet(s state.ReadOnlyBeaconState, a ethpb.SignedAggregateAttAndProof) (*bls.SignatureBatch, error) {
	aggregateAndProof := a.AggregateAttestationAndProof()

	v, err := s.ValidatorAtIndexReadOnly(aggregateAndProof.GetAggregatorIndex())
	if err != nil {
		return nil, err
	}
	pk := v.PublicKey()
	publicKey, err := bls.PublicKeyFromBytes(pk[:])
	if err != nil {
		return nil, err
	}

	epoch := slots.ToEpoch(aggregateAndProof.AggregateVal().GetData().Slot)
	d, err := signing.Domain(s.Fork(), epoch, params.BeaconConfig().DomainAggregateAndProof, s.GenesisValidatorsRoot())
	if err != nil {
		return nil, err
	}
	root, err := signing.ComputeSigningRoot(aggregateAndProof, d)
	if err != nil {
		return nil, err
	}
	return &bls.SignatureBatch{
		Signatures:   [][]byte{a.GetSignature()},
		PublicKeys:   []bls.PublicKey{publicKey},
		Messages:     [][32]byte{root},
		Descriptions: []string{signing.AggregatorSignature},
	}, nil
}
