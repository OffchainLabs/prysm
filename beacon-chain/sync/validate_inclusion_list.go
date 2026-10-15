package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// maxInclusionListsPerValidator is the number of valid inclusion lists that gossip forwards for one (slot, dependent_root, validator_index).
const maxInclusionListsPerValidator = 2

// validateInclusionListGossip validates a signed inclusion list on gossip, as in validate_inclusion_list_gossip.
// TODO(heze): run the beta.4 gossip_validation vectors when a runner exists and the EIP-8015 types land.
func (s *Service) validateInclusionListGossip(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateInclusionListGossip")
	defer span.End()

	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		return pubsub.ValidationReject, err
	}

	signed, ok := m.(*ethpb.SignedInclusionList)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	if signed.Message == nil {
		return pubsub.ValidationReject, errNilMessage
	}
	// [REJECT] The message is not larger than MAX_SIGNED_INCLUSION_LIST_SIZE.
	if size := signed.SizeSSZ(); size > fieldparams.MaxSignedInclusionListSize {
		return pubsub.ValidationReject, errors.Errorf("signed inclusion list is %d bytes, the maximum is %d", size, fieldparams.MaxSignedInclusionListSize)
	}
	il := signed.Message
	if len(il.DependentRoot) != fieldparams.RootLength {
		return pubsub.ValidationReject, blockchain.ErrInclusionListDependentRootLength
	}

	// [IGNORE] This is the first or second valid message from the validator for (slot, dependent_root, validator_index).
	key := inclusionListSeenKey(il)
	if s.inclusionListSeenCount(key) >= maxInclusionListsPerValidator {
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] The slot is the current slot, with MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance.
	current, err := s.cfg.clock.WithinSlotRange(il.Slot, il.Slot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if !current {
		return pubsub.ValidationIgnore, errors.Errorf("inclusion list slot %d is not the current slot %d", il.Slot, s.cfg.clock.CurrentSlot())
	}

	// OnInclusionList runs the other checks in spec order and stores the list only when all of them pass.
	if err := s.cfg.chain.OnInclusionList(ctx, signed); err != nil {
		return inclusionListValidationResult(err), err
	}

	if !s.incrementInclusionListSeenCount(il.Slot, key) {
		return pubsub.ValidationIgnore, nil
	}
	msg.ValidatorData = signed
	return pubsub.ValidationAccept, nil
}

// inclusionListSubscriber sends the inclusion list event. The validator already stored the list with OnInclusionList.
func (s *Service) inclusionListSubscriber(_ context.Context, msg proto.Message) error {
	signed, ok := msg.(*ethpb.SignedInclusionList)
	if !ok {
		return errWrongMessage
	}
	if signed.Message == nil {
		return errNilMessage
	}

	log.WithFields(logrus.Fields{
		"slot":           signed.Message.Slot,
		"validatorIndex": signed.Message.ValidatorIndex,
		"dependentRoot":  fmt.Sprintf("%#x", bytesutil.Trunc(signed.Message.DependentRoot)),
	}).Debug("Received inclusion list")

	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.InclusionListReceived,
		Data: &opfeed.InclusionListReceivedData{SignedInclusionList: signed},
	})
	return nil
}

// inclusionListValidationResult maps an OnInclusionList error to the gossip result of its spec check.
func inclusionListValidationResult(err error) pubsub.ValidationResult {
	switch {
	case errors.Is(err, blockchain.ErrNilInclusionList),
		errors.Is(err, blockchain.ErrInclusionListDependentRootLength),
		errors.Is(err, blockchain.ErrInclusionListTooLarge),
		errors.Is(err, blockchain.ErrInclusionListEmptyTransaction),
		errors.Is(err, blockchain.ErrInclusionListDependentBlockTooLate),
		errors.Is(err, blockchain.ErrInclusionListNotInCommittee),
		errors.Is(err, blockchain.ErrInclusionListInvalidSignature):
		return pubsub.ValidationReject
	default:
		return pubsub.ValidationIgnore
	}
}

func inclusionListSeenKey(il *ethpb.InclusionList) string {
	b := append(bytesutil.Bytes8(uint64(il.Slot)), il.DependentRoot...)
	return string(append(b, bytesutil.Bytes8(uint64(il.ValidatorIndex))...))
}

func (s *Service) inclusionListSeenCount(key string) int {
	v, ok := s.seenInclusionListCache.Get(key)
	if !ok {
		return 0
	}
	n, ok := v.(int)
	if !ok {
		return 0
	}
	return n
}

// incrementInclusionListSeenCount counts one more valid message for key. It returns false when the key already has the maximum count.
func (s *Service) incrementInclusionListSeenCount(slot primitives.Slot, key string) bool {
	s.seenInclusionListLock.Lock()
	defer s.seenInclusionListLock.Unlock()
	n := s.inclusionListSeenCount(key)
	if n >= maxInclusionListsPerValidator {
		return false
	}
	s.seenInclusionListCache.Add(slot, key, n+1)
	return true
}
