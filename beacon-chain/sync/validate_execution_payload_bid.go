package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

// validateExecutionPayloadBidGossip validates execution payload bids on gossip.
// The following validations MUST pass before forwarding the signed_execution_payload_bid
// on the network, assuming the alias bid = signed_execution_payload_bid.message:
func (s *Service) validateExecutionPayloadBidGossip(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateExecutionPayloadBidGossip")
	defer span.End()

	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		return pubsub.ValidationReject, err
	}

	signedBid, ok := m.(*ethpb.SignedExecutionPayloadBid)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	b, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	v := s.newExecutionPayloadBidVerifier(b, verification.GossipExecutionPayloadBidRequirements)
	bid, err := b.Bid()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [IGNORE] bid.slot is the current slot or the next slot.
	if err := v.VerifyCurrentOrNextSlot(); err != nil {
		return pubsub.ValidationIgnore, err
	}
	// [IGNORE] the SignedProposerPreferences where preferences.proposal_slot is equal to bid.slot has been seen.
	pref, ok := s.proposerPreferencesCache.Get(bid.Slot())
	if !ok {
		return pubsub.ValidationIgnore, nil
	}
	st, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	// [REJECT] bid.builder_index is a valid/active builder index.
	if err := v.VerifyBuilderActive(st); err != nil {
		return pubsub.ValidationReject, err
	}
	// [REJECT] bid.execution_payment is zero.
	if err := v.VerifyExecutionPaymentNonZero(); err != nil {
		return pubsub.ValidationReject, err
	}
	// [REJECT] bid.fee_recipient matches the fee_recipient from the proposer's SignedProposerPreferences associated with bid.slot.
	if err := v.VerifyFeeRecipientMatches(pref.FeeRecipient); err != nil {
		return pubsub.ValidationReject, err
	}
	// [REJECT] bid.gas_limit matches the gas_limit from the proposer's SignedProposerPreferences associated with bid.slot.
	if err := v.VerifyGasLimitMatches(pref.GasLimit); err != nil {
		return pubsub.ValidationReject, err
	}
	// The spec lists signature validation later, but the "first signed bid seen
	// with a valid signature" gate below depends on knowing validity first.
	if err := v.VerifySignature(st); err != nil {
		return pubsub.ValidationReject, err
	}

	// [IGNORE] this is the first signed bid seen with a valid signature from the given builder for this slot.
	builderKey := executionPayloadBidBuilderKey(bid.Slot(), bid.BuilderIndex())
	if s.hasSeenExecutionPayloadBidBuilder(builderKey) {
		return pubsub.ValidationIgnore, nil
	}
	s.setSeenExecutionPayloadBidBuilder(bid.Slot(), builderKey)
	// [IGNORE] this bid is the highest value bid seen for the tuple (bid.slot, bid.parent_block_hash, bid.parent_block_root).
	if !s.isHighestExecutionPayloadBid(bid) {
		return pubsub.ValidationIgnore, nil
	}
	// [IGNORE] bid.value is less or equal than the builder's excess balance.
	if err := v.VerifyBuilderCanCoverBid(st); err != nil {
		return pubsub.ValidationIgnore, err
	}
	// [IGNORE] bid.parent_block_hash is the block hash of a known execution payload in fork choice.
	if err := v.VerifyParentBlockHash(s.cfg.chain.BlockHash); err != nil {
		return pubsub.ValidationIgnore, err
	}
	// [IGNORE] bid.parent_block_root is the hash tree root of a known beacon block in fork choice.
	if err := v.VerifyParentBlockRootSeen(s.cfg.chain.InForkchoice); err != nil {
		return pubsub.ValidationIgnore, err
	}
	// [REJECT] signed_execution_payload_bid.signature is valid with respect to the bid.builder_index.
	// Verified earlier to satisfy the "first valid signed bid seen" condition.
	msg.ValidatorData = signedBid
	return pubsub.ValidationAccept, nil
}

func (s *Service) executionPayloadBidSubscriber(_ context.Context, msg proto.Message) error {
	signedBid, ok := msg.(*ethpb.SignedExecutionPayloadBid)
	if !ok {
		return errWrongMessage
	}
	if signedBid.Message == nil {
		return errNilMessage
	}
	s.setHighestExecutionPayloadBid(signedBid)
	return nil
}

func executionPayloadBidBuilderKey(slot primitives.Slot, builderIndex primitives.BuilderIndex) string {
	b := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(builderIndex))...)
	return string(b)
}

func (s *Service) hasSeenExecutionPayloadBidBuilder(key string) bool {
	_, seen := s.seenExecutionPayloadBidCache.Get(key)
	return seen
}

func (s *Service) setSeenExecutionPayloadBidBuilder(slot primitives.Slot, key string) {
	s.seenExecutionPayloadBidCache.Add(slot, key, true)
}

func (s *Service) isHighestExecutionPayloadBid(bid interfaces.ROExecutionPayloadBid) bool {
	cached, ok := s.highestExecutionPayloadBidCache.Get(bid.Slot(), bid.ParentBlockHash(), bid.ParentBlockRoot())
	if !ok {
		return true
	}
	return bid.Value() > cached.Message.Value
}

func (s *Service) setHighestExecutionPayloadBid(signedBid *ethpb.SignedExecutionPayloadBid) {
	s.highestExecutionPayloadBidCache.SetIfHigher(signedBid)
}
