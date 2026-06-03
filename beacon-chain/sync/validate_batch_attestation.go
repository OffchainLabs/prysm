// EIP-8243 batch attestation gossip validation. Lives alongside
// validate_beacon_attestation.go; the entry point dispatches here when the
// inner attestation unwrapped from a WireAttestation is a *BatchAttestation.
//
// The validation runs ALL the inherited single-attestation gossip checks
// (Codex fix #3) — slot window, target epoch, bad-block, block+state
// availability, forkchoice descent, LMD-FFG, subnet — and then the
// batch-specific checks: bitlist length, batcher membership, 3-signature BLS
// batch (data signature + aggregate seal + batcher composition signature) and
// the dedup rule "the message must add at least one previously unseen vote."

package sync

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

// batchAttSignatureBatchDesc is the signature-set description carried by
// validateWithBatchVerifier metrics for batch attestations. It groups all
// three signatures (data, seal, batcher) under a single label so the verifier
// can publish one duration histogram per batch.
const batchAttSignatureBatchDesc = "batch_attestation"

// batchDedupKey identifies a (slot, committee) duty. Per Codex fix #4, the
// EIP's data_root suffix is intentionally omitted: keying by data_root would
// let one attester re-spam across conflicting votes by varying the vote data,
// since each variation would land in a different bucket.
type batchDedupKey struct {
	slot           primitives.Slot
	committeeIndex primitives.CommitteeIndex
}

func (k batchDedupKey) bytes() string {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[0:8], uint64(k.slot))
	binary.LittleEndian.PutUint64(b[8:16], uint64(k.committeeIndex))
	return string(b)
}

// batchDedupEntry tracks accepted batch state for one duty.
//
//   - seenAttesters is committee-sized; bit i is set when the validator at
//     committee position i has been observed as part of any accepted batch.
//   - seenBatchers tracks which validators have already published an accepted
//     batch for this duty; a second batch from the same batcher is dropped.
//
// The mutex guards the seenAttesters bitlist OR and the seenBatchers map
// against concurrent gossip validations of the same duty.
type batchDedupEntry struct {
	mu            sync.Mutex
	seenAttesters bitfield.Bitlist
	seenBatchers  map[primitives.ValidatorIndex]struct{}
}

// getOrCreateBatchDedupEntry fetches (or initializes) the dedup state for a
// given duty. committeeSize is required to size the seenAttesters bitlist.
func (s *Service) getOrCreateBatchDedupEntry(key batchDedupKey, committeeSize uint64) *batchDedupEntry {
	s.seenBatchAttestationLock.Lock()
	defer s.seenBatchAttestationLock.Unlock()
	if raw, ok := s.seenBatchAttestationCache.Get(key.bytes()); ok {
		if e, ok := raw.(*batchDedupEntry); ok {
			return e
		}
	}
	e := &batchDedupEntry{
		seenAttesters: bitfield.NewBitlist(committeeSize),
		seenBatchers:  make(map[primitives.ValidatorIndex]struct{}, 2),
	}
	s.seenBatchAttestationCache.Add(key.bytes(), e)
	return e
}

func (s *Service) hasSeenBatchAttester(key batchDedupKey, committeeSize uint64, committeePosition uint64) bool {
	entry := s.getOrCreateBatchDedupEntry(key, committeeSize)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if committeePosition >= uint64(entry.seenAttesters.Len()) {
		return false
	}
	return entry.seenAttesters.BitAt(committeePosition)
}

func (s *Service) setSeenBatchAttester(key batchDedupKey, committeeSize uint64, committeePosition uint64) bool {
	entry := s.getOrCreateBatchDedupEntry(key, committeeSize)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if committeePosition >= uint64(entry.seenAttesters.Len()) {
		return false
	}
	if entry.seenAttesters.BitAt(committeePosition) {
		return false
	}
	entry.seenAttesters.SetBitAt(committeePosition, true)
	return true
}

func (s *Service) setSeenUnaggregatedAttesters(slot primitives.Slot, committeeIndex primitives.CommitteeIndex, attesters []primitives.ValidatorIndex) {
	for _, attester := range attesters {
		_ = s.setSeenUnaggregatedAtt(generateUnaggregatedAttCacheKeyForAttester(slot, committeeIndex, uint64(attester)))
	}
}

// validateBatchAttestation runs the full gossip validation pipeline for a
// post-EIP-8243 BatchAttestation. The caller must have already unwrapped the
// WireAttestation and confirmed the inner type.
func (s *Service) validateBatchAttestation(
	ctx context.Context,
	msg *pubsub.Message,
	batch *eth.BatchAttestation,
) (pubsub.ValidationResult, error) {
	ctx, span := trace.StartSpan(ctx, "sync.validateBatchAttestation")
	defer span.End()

	if batch == nil || batch.IsNil() {
		return pubsub.ValidationReject, errors.New("batch attestation is nil")
	}
	data := batch.GetData()

	// ----- Common gossip checks (Codex fix #3: applied identically to singles) -----

	// Slot 0 attestations are not processed at any phase.
	if data.Slot == 0 {
		return pubsub.ValidationIgnore, nil
	}
	if err := helpers.ValidateAttestationTime(data.Slot, s.cfg.clock.GenesisTime(), earlyAttestationProcessingTolerance); err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}
	if err := helpers.ValidateSlotTargetEpoch(data); err != nil {
		return pubsub.ValidationReject, wrapAttestationError(err, batch)
	}

	// EIP-8243 batch-specific REJECT: at least two attesters required.
	// (If only one validator is voting, use SingleAttestation instead.)
	if batch.AggregationBits.Count() < 2 {
		return pubsub.ValidationReject, wrapAttestationError(
			errors.New("batch attestation must have >= 2 attesters set"), batch)
	}

	if !s.slasherEnabled {
		// Reject if the batch references a block we have already marked bad.
		if s.hasBadBlock(bytesutil.ToBytes32(data.BeaconBlockRoot)) ||
			s.hasBadBlock(bytesutil.ToBytes32(data.Target.Root)) ||
			s.hasBadBlock(bytesutil.ToBytes32(data.Source.Root)) {
			attBadBlockCount.Inc()
			return pubsub.ValidationReject,
				wrapAttestationError(errors.New("batch references bad block root"), batch)
		}
	}

	// Block + state must be available; otherwise queue for later replay.
	blockRoot := bytesutil.ToBytes32(data.BeaconBlockRoot)
	if !s.hasBlockAndState(ctx, blockRoot) {
		s.savePendingAtt(batch)
		return pubsub.ValidationIgnore, nil
	}
	if !s.cfg.chain.InForkchoice(blockRoot) {
		tracing.AnnotateError(span, blockchain.ErrNotDescendantOfFinalized)
		return pubsub.ValidationIgnore, blockchain.ErrNotDescendantOfFinalized
	}
	if err := s.cfg.chain.VerifyLmdFfgConsistency(ctx, batch); err != nil {
		tracing.AnnotateError(span, err)
		attBadLmdConsistencyCount.Inc()
		return pubsub.ValidationReject, wrapAttestationError(err, batch)
	}

	preState, err := s.cfg.chain.AttestationTargetState(ctx, data.Target)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}

	// Subnet check: the topic must match the committee. Reused as-is from the
	// single-attestation path (Codex fix #3).
	validationRes, err := s.validateUnaggregatedAttTopic(ctx, batch, preState, *msg.Topic)
	if validationRes != pubsub.ValidationAccept {
		return validationRes, wrapAttestationError(err, batch)
	}

	committee, err := helpers.BeaconCommitteeFromState(ctx, preState, data.Slot, batch.CommitteeIndex)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}

	// Codex fix #5: REJECT if bitlist length does not equal committee size.
	// Without this, an overlong bitlist would otherwise be quietly truncated
	// and processed.
	if err := helpers.VerifyBitfieldLength(batch.AggregationBits, uint64(len(committee))); err != nil {
		return pubsub.ValidationReject, wrapAttestationError(err, batch)
	}

	// Resolve attesters from the bitlist; the batcher must be among them with
	// its committee-position bit set in aggregation_bits.
	attesterIndices := make([]primitives.ValidatorIndex, 0, batch.AggregationBits.Count())
	batcherInSet := false
	for i, vi := range committee {
		if batch.AggregationBits.BitAt(uint64(i)) {
			attesterIndices = append(attesterIndices, vi)
			if vi == batch.Batcher {
				batcherInSet = true
			}
		}
	}
	if !batcherInSet {
		return pubsub.ValidationReject,
			wrapAttestationError(errors.New("batcher not in attester set"), batch)
	}

	// ----- Dedup (cheap, before BLS) -----
	//
	// Spec rules:
	//   IGNORE if batcher ∈ seen_batchers for this duty.
	//   IGNORE if every attester in aggregation_bits is already in seen_attesters.
	key := batchDedupKey{slot: data.Slot, committeeIndex: batch.CommitteeIndex}
	entry := s.getOrCreateBatchDedupEntry(key, uint64(len(committee)))
	entry.mu.Lock()
	if _, seen := entry.seenBatchers[batch.Batcher]; seen {
		entry.mu.Unlock()
		batchAttDedupIgnoreCount.WithLabelValues("batcher_seen").Inc()
		return pubsub.ValidationIgnore, nil
	}
	if seenAttestersCoverAggregationBits(entry.seenAttesters, batch.AggregationBits) {
		entry.mu.Unlock()
		batchAttDedupIgnoreCount.WithLabelValues("attesters_covered").Inc()
		return pubsub.ValidationIgnore, nil
	}
	entry.mu.Unlock()

	// ----- BLS verification (3 signatures in one batch verifier set) -----
	sigSet, err := s.buildBatchAttestationSignatureSet(ctx, preState, batch, attesterIndices)
	if err != nil {
		tracing.AnnotateError(span, err)
		attBadSignatureBatchCount.Inc()
		return pubsub.ValidationReject, wrapAttestationError(err, batch)
	}
	validationRes, err = s.validateWithBatchVerifier(ctx, batchAttSignatureBatchDesc, sigSet)
	if validationRes != pubsub.ValidationAccept {
		return validationRes, err
	}

	// ----- Mark seen atomically -----
	entry.mu.Lock()
	// Re-check under lock; another goroutine could have marked the same batcher
	// between our pre-BLS check and now (TOCTOU window).
	if _, seen := entry.seenBatchers[batch.Batcher]; seen {
		entry.mu.Unlock()
		return pubsub.ValidationIgnore, nil
	}
	if seenAttestersCoverAggregationBits(entry.seenAttesters, batch.AggregationBits) {
		entry.mu.Unlock()
		return pubsub.ValidationIgnore, nil
	}
	entry.seenBatchers[batch.Batcher] = struct{}{}
	entry.seenAttesters = orBitlists(entry.seenAttesters, batch.AggregationBits, uint64(len(committee)))
	entry.mu.Unlock()
	s.setSeenUnaggregatedAttesters(data.Slot, batch.CommitteeIndex, attesterIndices)

	batchAttReceivedCount.Inc()
	batchAttAttestersHistogram.Observe(float64(batch.AggregationBits.Count()))

	// Notify other services in the beacon node. We piggy-back on the existing
	// single-attestation event feed since downstream consumers (slasher feed,
	// metrics) handle the stripped AttestationElectra uniformly.
	electraForm := batch.ToAttestationElectra()
	s.cfg.attestationNotifier.OperationFeed().Send(&feed.Event{
		Type: operation.UnaggregatedAttReceived,
		Data: &operation.UnAggregatedAttReceivedData{Attestation: electraForm},
	})

	// Hand the AttestationElectra (gossip-only seal/batcher fields stripped)
	// to the subscriber via msg.ValidatorData, matching the single path.
	msg.ValidatorData = electraForm
	return pubsub.ValidationAccept, nil
}

// seenAttestersCoverAggregationBits returns true when every bit set in
// aggregation_bits is also set in seen — implementing the spec's "every
// validator in aggregation_bits is in seen_attesters" IGNORE rule.
func seenAttestersCoverAggregationBits(seen, agg bitfield.Bitlist) bool {
	if len(seen) == 0 {
		// Zero seen attesters cannot cover any non-empty bitlist.
		return agg.Count() == 0
	}
	for i := uint64(0); i < agg.Len(); i++ {
		if agg.BitAt(i) && !seen.BitAt(i) {
			return false
		}
	}
	return true
}

// orBitlists OR-merges src into dst. If dst is shorter than the committee
// size (e.g. a freshly initialized entry whose committeeSize was incorrect),
// it is replaced with a fresh committee-sized bitlist before merging.
func orBitlists(dst, src bitfield.Bitlist, committeeSize uint64) bitfield.Bitlist {
	if uint64(dst.Len()) != committeeSize {
		dst = bitfield.NewBitlist(committeeSize)
	}
	for i := uint64(0); i < src.Len() && i < committeeSize; i++ {
		if src.BitAt(i) {
			dst.SetBitAt(i, true)
		}
	}
	return dst
}

// buildBatchAttestationSignatureSet packs the three BLS verifications a
// BatchAttestation requires into one *bls.SignatureBatch so the BLS batch
// verifier can amortize pairing operations across all three.
//
// All three signatures use a domain derived from the attestation's TARGET
// epoch (matching VerifyIndexedAttestation in attestation.go:259 — Codex
// fix #2), so the verifier can construct the signing root without referring
// to the inner state for each domain.
func (s *Service) buildBatchAttestationSignatureSet(
	ctx context.Context,
	bs state.ReadOnlyBeaconState,
	att *eth.BatchAttestation,
	attesterIndices []primitives.ValidatorIndex,
) (*bls.SignatureBatch, error) {
	if len(attesterIndices) < 2 {
		return nil, fmt.Errorf("batch attestation must have >= 2 attesters, got %d", len(attesterIndices))
	}

	fork := bs.Fork()
	gvr := bs.GenesisValidatorsRoot()
	targetEpoch := att.GetData().Target.Epoch

	// Decompress every attester's public key and aggregate them once. Used for
	// both the attestation-data signature and the batch-seal signature.
	attesterPubs := make([]bls.PublicKey, 0, len(attesterIndices))
	for _, vi := range attesterIndices {
		pkBytes := bs.PubkeyAtIndex(vi)
		pk, err := bls.PublicKeyFromBytes(pkBytes[:])
		if err != nil {
			return nil, errors.Wrap(err, "could not deserialize attester public key")
		}
		attesterPubs = append(attesterPubs, pk)
	}
	aggregateAttesterPub := bls.AggregateMultiplePubkeys(attesterPubs)

	// Batcher key (already in the attester set, but resolved separately so the
	// composition signature can be verified individually).
	batcherPubBytes := bs.PubkeyAtIndex(att.Batcher)
	batcherPub, err := bls.PublicKeyFromBytes(batcherPubBytes[:])
	if err != nil {
		return nil, errors.Wrap(err, "could not deserialize batcher public key")
	}

	set := bls.NewSet()

	// 1) Aggregate attestation signature over `att.data` under DomainBeaconAttester.
	//    Identical in domain and message to a standard aggregated attestation.
	attDomain, err := signing.Domain(fork, targetEpoch, params.BeaconConfig().DomainBeaconAttester, gvr)
	if err != nil {
		return nil, err
	}
	attRoot, err := signing.ComputeSigningRoot(att.GetData(), attDomain)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute signing root for attestation data")
	}
	set.Signatures = append(set.Signatures, att.Signature)
	set.PublicKeys = append(set.PublicKeys, aggregateAttesterPub)
	set.Messages = append(set.Messages, attRoot)
	set.Descriptions = append(set.Descriptions, "batch_att_data")

	// 2) Aggregate batch seal over BatchSealPreimage(slot, ci, batcher) under
	//    DomainBatchAttester. Codex fix #1: domain must be present (the EIP
	//    text omitted it).
	sealDomain, err := signing.Domain(fork, targetEpoch, params.BeaconConfig().DomainBatchAttester, gvr)
	if err != nil {
		return nil, err
	}
	sealPreimage := &eth.BatchSealPreimage{
		Slot:           att.GetData().Slot,
		CommitteeIndex: att.CommitteeIndex,
		Batcher:        att.Batcher,
	}
	sealRoot, err := signing.ComputeSigningRoot(sealPreimage, sealDomain)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute signing root for batch seal")
	}
	set.Signatures = append(set.Signatures, att.BatchSeal)
	set.PublicKeys = append(set.PublicKeys, aggregateAttesterPub)
	set.Messages = append(set.Messages, sealRoot)
	set.Descriptions = append(set.Descriptions, "batch_seal")

	// 3) Batcher's composition signature over BatcherPreimage(slot, ci,
	//    aggregation_bits, data_root) under DomainBatcher. Codex fix #6:
	//    data_root binds the composition to a specific AttestationData,
	//    preventing replay across conflicting votes.
	dataRoot, err := att.GetData().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not compute data hash tree root")
	}
	batcherDomain, err := signing.Domain(fork, targetEpoch, params.BeaconConfig().DomainBatcher, gvr)
	if err != nil {
		return nil, err
	}
	batcherPreimage := &eth.BatcherPreimage{
		Slot:            att.GetData().Slot,
		CommitteeIndex:  att.CommitteeIndex,
		AggregationBits: att.AggregationBits,
		DataRoot:        dataRoot[:],
	}
	batcherRoot, err := signing.ComputeSigningRoot(batcherPreimage, batcherDomain)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute signing root for batcher composition")
	}
	set.Signatures = append(set.Signatures, att.BatcherSignature)
	set.PublicKeys = append(set.PublicKeys, batcherPub)
	set.Messages = append(set.Messages, batcherRoot)
	set.Descriptions = append(set.Descriptions, "batcher_signature")

	return set, nil
}
