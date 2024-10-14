package sync

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	mockSync "github.com/prysmaticlabs/prysm/v5/beacon-chain/sync/initial-sync/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
)

func TestValidateExecutionPayloadHeader_IncorrectTopic(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
	s := &Service{
		cfg: &config{chain: chainService, p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}

	msg := random.ExecutionPayloadHeader(t)
	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestToTopic(topic, digest)

	result, err := s.validateExecutionPayloadHeader(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}})
	require.ErrorContains(t, "extraction failed for topic", err)
	require.Equal(t, result, pubsub.ValidationReject)
}

func TestValidateExecutionPayloadHeader_MockErrorPath(t *testing.T) {
	tests := []struct {
		error    error
		verifier verification.NewExecutionPayloadHeaderVerifier
		result   pubsub.ValidationResult
	}{
		{
			error: errors.New("incorrect slot"),
			verifier: func(e interfaces.ROSignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, reqs []verification.Requirement) verification.ExecutionPayloadHeaderVerifier {
				return &verification.MockExecutionPayloadHeader{ErrIncorrectSlot: errors.New("incorrect slot")}
			},
			result: pubsub.ValidationIgnore,
		},
		{
			error: errors.New("unknown block root"),
			verifier: func(e interfaces.ROSignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, reqs []verification.Requirement) verification.ExecutionPayloadHeaderVerifier {
				return &verification.MockExecutionPayloadHeader{ErrUnknownParentBlockRoot: errors.New("unknown block root")}
			},
			result: pubsub.ValidationIgnore,
		},
		{
			error: errors.New("unknown block hash"),
			verifier: func(e interfaces.ROSignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, reqs []verification.Requirement) verification.ExecutionPayloadHeaderVerifier {
				return &verification.MockExecutionPayloadHeader{ErrUnknownParentBlockHash: errors.New("unknown block hash")}
			},
			result: pubsub.ValidationIgnore,
		},
		{
			error: errors.New("invalid signature"),
			verifier: func(e interfaces.ROSignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, reqs []verification.Requirement) verification.ExecutionPayloadHeaderVerifier {
				return &verification.MockExecutionPayloadHeader{ErrInvalidSignature: errors.New("invalid signature")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("builder slashed"),
			verifier: func(e interfaces.ROSignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, reqs []verification.Requirement) verification.ExecutionPayloadHeaderVerifier {
				return &verification.MockExecutionPayloadHeader{ErrBuilderSlashed: errors.New("builder slashed")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("insufficient balance"),
			verifier: func(e interfaces.ROSignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, reqs []verification.Requirement) verification.ExecutionPayloadHeaderVerifier {
				return &verification.MockExecutionPayloadHeader{ErrBuilderInsufficientBalance: errors.New("insufficient balance")}
			},
			result: pubsub.ValidationReject,
		},
	}
	for _, tt := range tests {
		t.Run(tt.error.Error(), func(t *testing.T) {
			ctx := context.Background()
			p := p2ptest.NewTestP2P(t)
			chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
			s := &Service{
				executionPayloadHeaderCache: cache.NewExecutionPayloadHeaders(),
				cfg:                         &config{chain: chainService, p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}
			s.newExecutionPayloadHeaderVerifier = tt.verifier

			msg := random.SignedExecutionPayloadHeader(t)
			buf := new(bytes.Buffer)
			_, err := p.Encoding().EncodeGossip(buf, msg)
			require.NoError(t, err)

			topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
			digest, err := s.currentForkDigest()
			require.NoError(t, err)
			topic = s.addDigestToTopic(topic, digest)

			result, err := s.validateExecutionPayloadHeader(ctx, "", &pubsub.Message{
				Message: &pb.Message{
					Data:  buf.Bytes(),
					Topic: &topic,
				}})

			require.ErrorContains(t, tt.error.Error(), err)
			require.Equal(t, result, tt.result)
		})
	}
}

func TestValidateExecutionPayloadHeader_Accept(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
	s := &Service{
		executionPayloadHeaderCache: cache.NewExecutionPayloadHeaders(),
		cfg:                         &config{chain: chainService, p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}
	s.newExecutionPayloadHeaderVerifier = func(e interfaces.ROSignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, reqs []verification.Requirement) verification.ExecutionPayloadHeaderVerifier {
		return &verification.MockExecutionPayloadHeader{}
	}

	msg := random.SignedExecutionPayloadHeader(t)
	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestToTopic(topic, digest)

	result, err := s.validateExecutionPayloadHeader(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}})
	require.NoError(t, err)
	require.Equal(t, result, pubsub.ValidationAccept)
}

func TestValidateExecutionPayloadHeader_MoreThanOneSameBuilder(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
	s := &Service{
		executionPayloadHeaderCache: cache.NewExecutionPayloadHeaders(),
		cfg:                         &config{chain: chainService, p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}
	s.newExecutionPayloadHeaderVerifier = func(e interfaces.ROSignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, reqs []verification.Requirement) verification.ExecutionPayloadHeaderVerifier {
		return &verification.MockExecutionPayloadHeader{}
	}

	msg := random.SignedExecutionPayloadHeader(t)
	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestToTopic(topic, digest)

	result, err := s.validateExecutionPayloadHeader(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}})
	require.NoError(t, err)
	require.Equal(t, result, pubsub.ValidationAccept)

	result, err = s.validateExecutionPayloadHeader(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}})
	require.ErrorContains(t, fmt.Sprintf("builder %d has already been seen in slot %d", msg.Message.BuilderIndex, msg.Message.Slot), err)
	require.Equal(t, result, pubsub.ValidationIgnore)
}

func TestValidateExecutionPayloadHeader_LowerValue(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
	s := &Service{
		executionPayloadHeaderCache: cache.NewExecutionPayloadHeaders(),
		cfg:                         &config{chain: chainService, p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}
	s.newExecutionPayloadHeaderVerifier = func(e interfaces.ROSignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, reqs []verification.Requirement) verification.ExecutionPayloadHeaderVerifier {
		return &verification.MockExecutionPayloadHeader{}
	}

	msg := random.SignedExecutionPayloadHeader(t)
	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestToTopic(topic, digest)

	m := &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}}
	result, err := s.validateExecutionPayloadHeader(ctx, "", m)
	require.NoError(t, err)
	require.Equal(t, result, pubsub.ValidationAccept)

	require.NoError(t, s.subscribeExecutionPayloadHeader(ctx, msg))

	// Different builder but lower value should fail
	newMsg := eth.CopySignedExecutionPayloadHeader(msg)
	newMsg.Message.BuilderIndex = newMsg.Message.BuilderIndex - 1
	newMsg.Message.Value = newMsg.Message.Value - 1
	newBuf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(newBuf, newMsg)
	require.NoError(t, err)

	result, err = s.validateExecutionPayloadHeader(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  newBuf.Bytes(),
			Topic: &topic,
		}})
	require.ErrorContains(t, "received header has lower value than cached header", err)
	require.Equal(t, result, pubsub.ValidationIgnore)
}

func TestAddAndSeenBuilderBySlot(t *testing.T) {
	resetBuilderBySlot()

	// Add builder to slot 1
	addBuilderBySlot(1, 100)
	require.Equal(t, true, seenBuilderBySlot(1, 100), "Builder 100 should be seen in slot 1")
	require.Equal(t, false, seenBuilderBySlot(1, 101), "Builder 101 should not be seen in slot 1")

	// Add builder to slot 2
	addBuilderBySlot(2, 200)
	require.Equal(t, true, seenBuilderBySlot(2, 200), "Builder 200 should be seen in slot 2")

	// Slot 3 should not have any builders yet
	require.Equal(t, false, seenBuilderBySlot(3, 300), "Builder 300 should not be seen in slot 3")

	// Add builder to slot 3
	addBuilderBySlot(3, 300)
	require.Equal(t, true, seenBuilderBySlot(3, 300), "Builder 300 should be seen in slot 3")

	// Now slot 1 should be removed (assuming the current slot is 3)
	require.Equal(t, false, seenBuilderBySlot(1, 100), "Builder 100 should no longer be seen in slot 1")

	// Slot 2 should still be valid
	require.Equal(t, true, seenBuilderBySlot(2, 200), "Builder 200 should still be seen in slot 2")

	// Add builder to slot 4, slot 2 should now be removed
	addBuilderBySlot(4, 400)
	require.Equal(t, true, seenBuilderBySlot(4, 400), "Builder 400 should be seen in slot 4")
	require.Equal(t, false, seenBuilderBySlot(2, 200), "Builder 200 should no longer be seen in slot 2")
}

func resetBuilderBySlot() {
	builderBySlotLock.Lock()
	defer builderBySlotLock.Unlock()
	builderBySlot = make(map[primitives.Slot]map[primitives.ValidatorIndex]struct{})
}
