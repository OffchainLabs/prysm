package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// validateInclusionList validates an incoming inclusion list message.
// Returns appropriate validation results based on the following rules:
// [REJECT] The slot `message.slot` is equal to the previous or current slot.
// [IGNORE] The slot `message.slot` is equal to the current slot, or it is equal to the previous slot and the current time
//
//	is less than attestation_deadline seconds into the slot.
//
// [IGNORE] The inclusion_list_committee for slot `message.slot` on the current branch corresponds to `message.inclusion_list_committee_root`,
//
//	as determined by `hash_tree_root(inclusion_list_committee) == message.inclusion_list_committee_root`.
//
// [REJECT] The validator index `message.validator_index` is within the inclusion_list_committee corresponding to `message.inclusion_list_committee_root`.
// [REJECT] The transactions `message.transactions` length is within the upper bound MAX_TRANSACTIONS_PER_INCLUSION_LIST.
// [IGNORE] The message is either the first or second valid message received from the validator with index `message.validator_index`.
// [REJECT] The signature of `inclusion_list.signature` is valid with respect to the validator index.
func (s *Service) validateInclusionList(ctx context.Context, id peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Skip self-published messages.
	if id == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Ignore if the node is currently syncing.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	// Validate topic presence.
	if msg.Topic == nil {
		return pubsub.ValidationReject, errInvalidTopic
	}

	// Decode the pubsub message into the appropriate type.
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		return pubsub.ValidationReject, err
	}
	il, ok := m.(*eth.SignedInclusionList)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}

	// Check for nil inclusion list.
	if err := helpers.ValidateNilSignedInclusionList(il); err != nil {
		return pubsub.ValidationIgnore, err
	}

	// Validate slot constraints.
	currentSlot := s.cfg.clock.CurrentSlot()
	if il.Message.Slot != currentSlot && il.Message.Slot+1 != currentSlot {
		return pubsub.ValidationReject, errors.New("slot %d is not equal to the previous %d or current %d slot")
	}
	secondsSinceSlotStart, err := slots.SecondsSinceSlotStart(currentSlot, uint64(s.cfg.chain.GenesisTime().Unix()), uint64(prysmTime.Now().Unix()))
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	deadline := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	if il.Message.Slot+1 == currentSlot && secondsSinceSlotStart > deadline {
		return pubsub.ValidationIgnore, errors.New("slot is equal to the previous slot and the current time is more than attestation_deadline seconds into the slot")
	}

	// Fetch the current head state.
	st, err := s.cfg.chain.HeadState(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	// Validate inclusion list committee root.
	committee, err := helpers.GetInclusionListCommittee(ctx, st, il.Message.Slot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	root, err := ssz.InclusionListRoot(committee)
	if err != nil {
		return pubsub.ValidationReject, err
	}
	if root != [32]byte(il.Message.InclusionListCommitteeRoot) {
		return pubsub.ValidationReject, errors.New("inclusion_list_committee_root does not match the inclusion_list_committee")
	}

	// Validate validator index is within the committee.
	var included bool
	for _, i := range committee {
		if i == il.Message.ValidatorIndex {
			included = true
			break
		}
	}
	if !included {
		return pubsub.ValidationReject, errors.New("validator_index is not within the inclusion_list_committee")
	}

	// Validate transaction size.
	totalSize := 0
	for _, transaction := range il.Message.Transactions {
		totalSize += len(transaction)
	}
	if totalSize > 8*1024 {
		return pubsub.ValidationReject, errors.New("total size of transactions exceeds 8KB")
	}

	// Check for duplicate inclusion list from the validator.
	if s.inclusionLists.SeenTwice(il.Message.Slot, il.Message.ValidatorIndex) {
		return pubsub.ValidationReject, errors.New("inclusion list seen twice")
	}

	// Validate the inclusion list signature.
	if err := helpers.ValidateInclusionListSignature(ctx, st, il); err != nil {
		return pubsub.ValidationReject, err
	}

	msg.ValidatorData = il

	return pubsub.ValidationAccept, nil
}

// subscriberInclusionList handles incoming inclusion list messages by adding them to the local inclusion list cache.
func (s *Service) subscriberInclusionList(ctx context.Context, msg proto.Message) error {
	il, ok := msg.(*eth.SignedInclusionList)
	if !ok {
		return fmt.Errorf("message was not type *ethpb.SignedInclusionList, type=%T", msg)
	}
	if il == nil {
		return errors.New("nil inclusion list")
	}

	isBeforeFreezeDeadline := s.cfg.clock.CurrentSlot() == il.Message.Slot &&
		slots.TimeIntoSlot(uint64(s.cfg.clock.GenesisTime().Unix())) < time.Duration(params.BeaconConfig().InclusionListFreezeDeadLine)*time.Second

	s.inclusionLists.Add(il.Message.Slot, il.Message.ValidatorIndex, il.Message.Transactions, isBeforeFreezeDeadline)

	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.InclusionListReceived,
		Data: &opfeed.InclusionListReceivedData{
			SignedInclusionList: il,
		},
	})

	return nil
}
