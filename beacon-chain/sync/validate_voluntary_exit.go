package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Clients who receive a voluntary exit on this topic MUST validate the conditions within process_voluntary_exit before
// forwarding it across the network.
func (s *Service) validateVoluntaryExit(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Validation runs on publish (not just subscriptions), so we should approve any message from
	// ourselves.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// The head state will be too far away to validate any voluntary exit.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateVoluntaryExit")
	defer span.End()

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	exit, ok := m.(*ethpb.SignedVoluntaryExit)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}

	if exit.Exit == nil {
		return pubsub.ValidationReject, errNilMessage
	}
	if s.hasSeenExitIndex(exit.Exit.ValidatorIndex) {
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] The voluntary exit epoch is not in the future.
	now := s.cfg.clock.Now().Add(params.BeaconConfig().MaximumGossipClockDisparityDuration())
	if currentEpoch := slots.ToEpoch(slots.At(s.cfg.clock.GenesisTime(), now)); currentEpoch < exit.Exit.Epoch {
		return pubsub.ValidationIgnore, fmt.Errorf("voluntary exit epoch %d is in the future, current epoch %d", exit.Exit.Epoch, currentEpoch)
	}

	headState, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [Modified in Gloas:EIP8282] Builder exits are no longer carried as voluntary
	// exits; builders exit via BuilderExitRequest on the execution layer.
	if exit.Exit.ValidatorIndex.IsBuilderIndex() {
		return pubsub.ValidationReject, errors.New("builder voluntary exits are not supported")
	}
	if uint64(exit.Exit.ValidatorIndex) >= uint64(headState.NumValidators()) {
		return pubsub.ValidationReject, errors.New("validator index is invalid")
	}
	val, err := headState.ValidatorAtIndexReadOnly(exit.Exit.ValidatorIndex)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	// [IGNORE] The validator has not already initiated exit.
	if val.ExitEpoch() != params.BeaconConfig().FarFutureEpoch {
		return pubsub.ValidationIgnore, fmt.Errorf("validator with index %d %s: %v", exit.Exit.ValidatorIndex, blocks.ValidatorAlreadyExitedMsg, val.ExitEpoch())
	}
	if err := blocks.VerifyExitAndSignatureWithoutEpochCheck(val, headState, exit); err != nil {
		// [IGNORE] The validator has no pending partial withdrawals.
		if errors.Is(err, blocks.ErrPendingBalanceToWithdraw) {
			return pubsub.ValidationIgnore, err
		}
		return pubsub.ValidationReject, err
	}

	msg.ValidatorData = exit // Used in downstream subscriber

	// Broadcast the voluntary exit on a feed to notify other services in the beacon node
	// of a received voluntary exit.
	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ExitReceived,
		Data: &opfeed.ExitReceivedData{
			Exit: exit,
		},
	})

	return pubsub.ValidationAccept, nil
}

// Returns true if the node has already received a valid exit request for the validator with index `i`.
func (s *Service) hasSeenExitIndex(i primitives.ValidatorIndex) bool {
	s.seenExitLock.RLock()
	defer s.seenExitLock.RUnlock()
	_, seen := s.seenExitCache.Get(i)
	return seen
}

// Set exit request index `i` in seen exit request cache.
func (s *Service) setExitIndexSeen(i primitives.ValidatorIndex) {
	s.seenExitLock.Lock()
	defer s.seenExitLock.Unlock()
	s.seenExitCache.Add(i, true)
}
