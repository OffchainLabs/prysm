package sync

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/operation"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/slasher/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// Validation
// - The block being voted for (attestation.data.beacon_block_root) passes validation.
// - The attestation's committee index (attestation.data.index) is for the correct subnet.
// - The attestation is unaggregated -- that is, it has exactly one participating validator (len(get_attesting_indices(state, attestation.data, attestation.aggregation_bits)) == 1).
// - attestation.data.slot is within the last ATTESTATION_PROPAGATION_SLOT_RANGE slots (attestation.data.slot + ATTESTATION_PROPAGATION_SLOT_RANGE >= current_slot >= attestation.data.slot).
// - The signature of attestation is valid.
func (s *Service) validateCommitteeIndexBeaconAttestation(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	// Attestation processing requires the target block to be present in the database, so we'll skip
	// validating or processing attestations until fully synced.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateCommitteeIndexBeaconAttestation")
	defer span.End()

	if msg.Topic == nil {
		return pubsub.ValidationReject, errInvalidTopic
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	att, ok := m.(eth.Att)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	if err := helpers.ValidateNilAttestation(att); err != nil {
		return pubsub.ValidationReject, err
	}
	data := att.GetData()

	// Do not process slot 0 attestations.
	if data.Slot == 0 {
		return pubsub.ValidationIgnore, nil
	}
	// Broadcast the unaggregated attestation on a feed to notify other services in the beacon node
	// of a received unaggregated attestation.
	s.cfg.attestationNotifier.OperationFeed().Send(&feed.Event{
		Type: operation.UnaggregatedAttReceived,
		Data: &operation.UnAggregatedAttReceivedData{
			Attestation: att,
		},
	})

	// Attestation's slot is within ATTESTATION_PROPAGATION_SLOT_RANGE and early attestation
	// processing tolerance.
	if err := helpers.ValidateAttestationTime(data.Slot, s.cfg.clock.GenesisTime(),
		earlyAttestationProcessingTolerance); err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}
	if err := helpers.ValidateSlotTargetEpoch(data); err != nil {
		return pubsub.ValidationReject, err
	}

	var validationRes pubsub.ValidationResult

	committeeIndex, result, err := s.validateCommitteeIndex(ctx, att)
	if result != pubsub.ValidationAccept {
		wrappedErr := errors.Wrapf(err, "could not validate committee index for %s version", version.String(att.Version()))
		tracing.AnnotateError(span, wrappedErr)
		return result, wrappedErr
	}

	if !features.Get().EnableSlasher {
		// Verify this the first attestation received for the participating validator for the slot.
		if s.hasSeenCommitteeIndicesSlot(data.Slot, committeeIndex, att.GetAggregationBits()) {
			return pubsub.ValidationIgnore, nil
		}

		// Reject an attestation if it references an invalid block.
		if s.hasBadBlock(bytesutil.ToBytes32(data.BeaconBlockRoot)) ||
			s.hasBadBlock(bytesutil.ToBytes32(data.Target.Root)) ||
			s.hasBadBlock(bytesutil.ToBytes32(data.Source.Root)) {
			attBadBlockCount.Inc()
			return pubsub.ValidationReject, errors.New("attestation data references bad block root")
		}
	}

	// Verify the block being voted and the processed state is in beaconDB and the block has passed validation if it's in the beaconDB.
	blockRoot := bytesutil.ToBytes32(data.BeaconBlockRoot)
	if !s.hasBlockAndState(ctx, blockRoot) {
		// A node doesn't have the block, it'll request from peer while saving the pending attestation to a queue.
		if att.Version() >= version.Electra {
			a, ok := att.(*eth.AttestationElectra)
			// This will never fail in practice because we asserted the version
			if !ok {
				return pubsub.ValidationIgnore, fmt.Errorf("attestation has wrong type (expected %T, got %T)", &eth.AttestationElectra{}, att)
			}
			s.savePendingAtt(&eth.SignedAggregateAttestationAndProofElectra{Message: &eth.AggregateAttestationAndProofElectra{Aggregate: a}})
		} else {
			a, ok := att.(*eth.Attestation)
			// This will never fail in practice because we asserted the version
			if !ok {
				return pubsub.ValidationIgnore, fmt.Errorf("attestation has wrong type (expected %T, got %T)", &eth.Attestation{}, att)
			}
			s.savePendingAtt(&eth.SignedAggregateAttestationAndProof{Message: &eth.AggregateAttestationAndProof{Aggregate: a}})
		}
		return pubsub.ValidationIgnore, nil
	}

	if !s.cfg.chain.InForkchoice(bytesutil.ToBytes32(data.BeaconBlockRoot)) {
		tracing.AnnotateError(span, blockchain.ErrNotDescendantOfFinalized)
		return pubsub.ValidationIgnore, blockchain.ErrNotDescendantOfFinalized
	}
	if err := s.cfg.chain.VerifyLmdFfgConsistency(ctx, att); err != nil {
		tracing.AnnotateError(span, err)
		attBadLmdConsistencyCount.Inc()
		return pubsub.ValidationReject, err
	}

	preState, err := s.cfg.chain.AttestationTargetState(ctx, data.Target)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}

	validationRes, err = s.validateUnaggregatedAttTopic(ctx, att, preState, *msg.Topic)
	if validationRes != pubsub.ValidationAccept {
		return validationRes, err
	}

	validationRes, err = s.validateUnaggregatedAttWithState(ctx, att, preState)
	if validationRes != pubsub.ValidationAccept {
		return validationRes, err
	}

	if features.Get().EnableSlasher {
		// Feed the indexed attestation to slasher if enabled. This action
		// is done in the background to avoid adding more load to this critical code path.
		go func() {
			// Using a different context to prevent timeouts as this operation can be expensive
			// and we want to avoid affecting the critical code path.
			ctx := context.TODO()
			preState, err := s.cfg.chain.AttestationTargetState(ctx, data.Target)
			if err != nil {
				log.WithError(err).Error("Could not retrieve pre state")
				tracing.AnnotateError(span, err)
				return
			}
			committee, err := helpers.BeaconCommitteeFromState(ctx, preState, data.Slot, committeeIndex)
			if err != nil {
				log.WithError(err).Error("Could not get attestation committee")
				tracing.AnnotateError(span, err)
				return
			}
			indexedAtt, err := attestation.ConvertToIndexed(ctx, att, committee)
			if err != nil {
				log.WithError(err).Error("Could not convert to indexed attestation")
				tracing.AnnotateError(span, err)
				return
			}
			s.cfg.slasherAttestationsFeed.Send(&types.WrappedIndexedAtt{IndexedAtt: indexedAtt})
		}()
	}

	s.setSeenCommitteeIndicesSlot(data.Slot, committeeIndex, att.GetAggregationBits())

	msg.ValidatorData = att

	return pubsub.ValidationAccept, nil
}

// This validates beacon unaggregated attestation has correct topic string.
func (s *Service) validateUnaggregatedAttTopic(ctx context.Context, a eth.Att, bs state.ReadOnlyBeaconState, t string) (pubsub.ValidationResult, error) {
	ctx, span := trace.StartSpan(ctx, "sync.validateUnaggregatedAttTopic")
	defer span.End()

	_, valCount, result, err := s.validateCommitteeIndexAndCount(ctx, a, bs)
	if result != pubsub.ValidationAccept {
		return result, err
	}
	subnet := helpers.ComputeSubnetForAttestation(valCount, a)
	format := p2p.GossipTypeMapping[reflect.TypeOf(&eth.Attestation{})]
	digest, err := s.currentForkDigest()
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}
	if !strings.HasPrefix(t, fmt.Sprintf(format, digest, subnet)) {
		return pubsub.ValidationReject, errors.New("attestation's subnet does not match with pubsub topic")
	}

	return pubsub.ValidationAccept, nil
}

func (s *Service) validateCommitteeIndexAndCount(
	ctx context.Context,
	a eth.Att,
	bs state.ReadOnlyBeaconState,
) (primitives.CommitteeIndex, uint64, pubsub.ValidationResult, error) {
	ci, result, err := s.validateCommitteeIndex(ctx, a)
	if result != pubsub.ValidationAccept {
		return 0, 0, result, err
	}
	valCount, err := helpers.ActiveValidatorCount(ctx, bs, slots.ToEpoch(a.GetData().Slot))
	if err != nil {
		return 0, 0, pubsub.ValidationIgnore, err
	}
	count := helpers.SlotCommitteeCount(valCount)
	if uint64(ci) > count {
		return 0, 0, pubsub.ValidationReject, fmt.Errorf("committee index %d > %d", a.GetData().CommitteeIndex, count)
	}
	return ci, valCount, pubsub.ValidationAccept, nil
}

func (s *Service) validateCommitteeIndex(ctx context.Context, a eth.Att) (primitives.CommitteeIndex, pubsub.ValidationResult, error) {
	if a.Version() >= version.Electra {
		return validateCommitteeIndexElectra(ctx, a)
	}
	return a.GetData().CommitteeIndex, pubsub.ValidationAccept, nil
}

// This validates beacon unaggregated attestation using the given state, the validation consists of bitfield length and count consistency
// and signature verification.
func (s *Service) validateUnaggregatedAttWithState(ctx context.Context, a eth.Att, bs state.ReadOnlyBeaconState) (pubsub.ValidationResult, error) {
	ctx, span := trace.StartSpan(ctx, "sync.validateUnaggregatedAttWithState")
	defer span.End()

	committeeIndex, err := a.GetCommitteeIndex()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	committee, result, err := s.validateBitLength(ctx, bs, a.GetData().Slot, committeeIndex, a.GetAggregationBits())
	if result != pubsub.ValidationAccept {
		return result, err
	}

	// Attestation must be unaggregated and the bit index must exist in the range of committee indices.
	// Note: The Ethereum Beacon chain spec suggests (len(get_attesting_indices(state, attestation.data, attestation.aggregation_bits)) == 1)
	// however this validation can be achieved without use of get_attesting_indices which is an O(n) lookup.
	if a.GetAggregationBits().Count() != 1 || a.GetAggregationBits().BitIndices()[0] >= len(committee) {
		return pubsub.ValidationReject, errors.New("attestation bitfield is invalid")
	}

	set, err := blocks.AttestationSignatureBatch(ctx, bs, []eth.Att{a})
	if err != nil {
		tracing.AnnotateError(span, err)
		attBadSignatureBatchCount.Inc()
		return pubsub.ValidationReject, err
	}
	return s.validateWithBatchVerifier(ctx, "attestation", set)
}

func (s *Service) validateBitLength(
	ctx context.Context,
	bs state.ReadOnlyBeaconState,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
	aggregationBits bitfield.Bitlist,
) ([]primitives.ValidatorIndex, pubsub.ValidationResult, error) {
	committee, err := helpers.BeaconCommitteeFromState(ctx, bs, slot, committeeIndex)
	if err != nil {
		return nil, pubsub.ValidationIgnore, err
	}

	// Verify number of aggregation bits matches the committee size.
	if err := helpers.VerifyBitfieldLength(aggregationBits, uint64(len(committee))); err != nil {
		return nil, pubsub.ValidationReject, err
	}

	return committee, pubsub.ValidationAccept, nil
}

// Returns true if the attestation was already seen for the participating validator for the slot.
func (s *Service) hasSeenCommitteeIndicesSlot(slot primitives.Slot, committeeID primitives.CommitteeIndex, aggregateBits []byte) bool {
	s.seenUnAggregatedAttestationLock.RLock()
	defer s.seenUnAggregatedAttestationLock.RUnlock()
	b := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(committeeID))...)
	b = append(b, aggregateBits...)
	_, seen := s.seenUnAggregatedAttestationCache.Get(string(b))
	return seen
}

// Set committee's indices and slot as seen for incoming attestations.
func (s *Service) setSeenCommitteeIndicesSlot(slot primitives.Slot, committeeID primitives.CommitteeIndex, aggregateBits []byte) {
	s.seenUnAggregatedAttestationLock.Lock()
	defer s.seenUnAggregatedAttestationLock.Unlock()
	b := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(committeeID))...)
	b = append(b, bytesutil.SafeCopyBytes(aggregateBits)...)
	s.seenUnAggregatedAttestationCache.Add(string(b), true)
}

// hasBlockAndState returns true if the beacon node knows about a block and associated state in the
// database or cache.
func (s *Service) hasBlockAndState(ctx context.Context, blockRoot [32]byte) bool {
	hasStateSummary := s.cfg.beaconDB.HasStateSummary(ctx, blockRoot)
	hasState := hasStateSummary || s.cfg.beaconDB.HasState(ctx, blockRoot)
	return hasState && s.cfg.chain.HasBlock(ctx, blockRoot)
}
