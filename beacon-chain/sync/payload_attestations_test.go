package sync

import (
	"bytes"
	"context"
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
	mockSync "github.com/prysmaticlabs/prysm/v5/beacon-chain/sync/initial-sync/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	payloadattestation "github.com/prysmaticlabs/prysm/v5/consensus-types/epbs/payload-attestation"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
)

func TestValidatePayloadAttestationMessage_IncorrectTopic(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
	s := &Service{
		payloadAttestationCache: &cache.PayloadAttestationCache{},
		cfg:                     &config{chain: chainService, p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}

	msg := random.PayloadAttestation(t) // Using payload attestation for message should fail.
	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestToTopic(topic, digest)

	result, err := s.validatePayloadAttestation(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}})
	require.ErrorContains(t, "extraction failed for topic", err)
	require.Equal(t, result, pubsub.ValidationReject)
}

func TestValidatePayloadAttestationMessage_ErrorPathsWithMock(t *testing.T) {
	tests := []struct {
		error    error
		verifier verification.NewPayloadAttestationMsgVerifier
		result   pubsub.ValidationResult
	}{
		{
			error: errors.New("incorrect slot"),
			verifier: func(pa payloadattestation.ROMessage, reqs []verification.Requirement) verification.PayloadAttestationMsgVerifier {
				return &verification.MockPayloadAttestation{ErrIncorrectPayloadAttSlot: errors.New("incorrect slot")}
			},
			result: pubsub.ValidationIgnore,
		},
		{
			error: errors.New("incorrect status"),
			verifier: func(pa payloadattestation.ROMessage, reqs []verification.Requirement) verification.PayloadAttestationMsgVerifier {
				return &verification.MockPayloadAttestation{ErrIncorrectPayloadAttStatus: errors.New("incorrect status")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("block root seen"),
			verifier: func(pa payloadattestation.ROMessage, reqs []verification.Requirement) verification.PayloadAttestationMsgVerifier {
				return &verification.MockPayloadAttestation{ErrPayloadAttBlockRootNotSeen: errors.New("block root seen")}
			},
			result: pubsub.ValidationIgnore,
		},
		{
			error: errors.New("block root invalid"),
			verifier: func(pa payloadattestation.ROMessage, reqs []verification.Requirement) verification.PayloadAttestationMsgVerifier {
				return &verification.MockPayloadAttestation{ErrPayloadAttBlockRootInvalid: errors.New("block root invalid")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("validator not in PTC"),
			verifier: func(pa payloadattestation.ROMessage, reqs []verification.Requirement) verification.PayloadAttestationMsgVerifier {
				return &verification.MockPayloadAttestation{ErrIncorrectPayloadAttValidator: errors.New("validator not in PTC")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("incorrect signature"),
			verifier: func(pa payloadattestation.ROMessage, reqs []verification.Requirement) verification.PayloadAttestationMsgVerifier {
				return &verification.MockPayloadAttestation{ErrInvalidMessageSignature: errors.New("incorrect signature")}
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
				payloadAttestationCache: &cache.PayloadAttestationCache{},
				cfg:                     &config{chain: chainService, p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}
			s.newPayloadAttestationVerifier = tt.verifier

			msg := random.PayloadAttestationMessage(t)
			buf := new(bytes.Buffer)
			_, err := p.Encoding().EncodeGossip(buf, msg)
			require.NoError(t, err)

			topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
			digest, err := s.currentForkDigest()
			require.NoError(t, err)
			topic = s.addDigestToTopic(topic, digest)

			result, err := s.validatePayloadAttestation(ctx, "", &pubsub.Message{
				Message: &pb.Message{
					Data:  buf.Bytes(),
					Topic: &topic,
				}})

			require.ErrorContains(t, tt.error.Error(), err)
			require.Equal(t, result, tt.result)
		})
	}
}

func TestValidatePayloadAttestationMessage_Accept(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
	s := &Service{
		payloadAttestationCache: &cache.PayloadAttestationCache{},
		cfg:                     &config{chain: chainService, p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}
	s.newPayloadAttestationVerifier = func(pa payloadattestation.ROMessage, reqs []verification.Requirement) verification.PayloadAttestationMsgVerifier {
		return &verification.MockPayloadAttestation{}
	}

	msg := random.PayloadAttestationMessage(t)
	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestToTopic(topic, digest)

	result, err := s.validatePayloadAttestation(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}})
	require.NoError(t, err)
	require.Equal(t, result, pubsub.ValidationAccept)
}
