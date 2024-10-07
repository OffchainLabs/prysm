package sync

import (
	"bytes"
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	testDB "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	doublylinkedtree "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stategen"
	mockSync "github.com/prysmaticlabs/prysm/v5/beacon-chain/sync/initial-sync/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
)

func TestValidateExecutionPayloadEnvelope_ErrorPathsWithMock(t *testing.T) {
	tests := []struct {
		error    error
		verifier verification.NewExecutionPayloadEnvelopeVerifier
		result   pubsub.ValidationResult
	}{
		{
			error: errors.New("block root seen"),
			verifier: func(e interfaces.ROSignedExecutionPayloadEnvelope, reqs []verification.Requirement) verification.ExecutionPayloadEnvelopeVerifier {
				return &verification.MockExecutionPayloadEnvelope{ErrBlockRootNotSeen: errors.New("block root seen")}
			},
			result: pubsub.ValidationIgnore,
		},
		{
			error: errors.New("block root invalid"),
			verifier: func(e interfaces.ROSignedExecutionPayloadEnvelope, reqs []verification.Requirement) verification.ExecutionPayloadEnvelopeVerifier {
				return &verification.MockExecutionPayloadEnvelope{ErrBlockRootInvalid: errors.New("block root invalid")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("invalid builder index"),
			verifier: func(e interfaces.ROSignedExecutionPayloadEnvelope, reqs []verification.Requirement) verification.ExecutionPayloadEnvelopeVerifier {
				return &verification.MockExecutionPayloadEnvelope{ErrBuilderIndexInvalid: errors.New("invalid builder index")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("invalid block hash"),
			verifier: func(e interfaces.ROSignedExecutionPayloadEnvelope, reqs []verification.Requirement) verification.ExecutionPayloadEnvelopeVerifier {
				return &verification.MockExecutionPayloadEnvelope{ErrBlockHashInvalid: errors.New("invalid block hash")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("incorrect signature"),
			verifier: func(e interfaces.ROSignedExecutionPayloadEnvelope, reqs []verification.Requirement) verification.ExecutionPayloadEnvelopeVerifier {
				return &verification.MockExecutionPayloadEnvelope{ErrSignatureInvalid: errors.New("incorrect signature")}
			},
			result: pubsub.ValidationReject,
		},
	}
	for _, tt := range tests {
		t.Run(tt.error.Error(), func(t *testing.T) {
			ctx := context.Background()
			db := testDB.SetupDB(t)
			fcs := doublylinkedtree.New()
			sg := stategen.New(db, fcs)
			p := p2ptest.NewTestP2P(t)
			chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}

			s := &Service{
				payloadEnvelopeCache: &sync.Map{},
				cfg: &config{
					chain:       chainService,
					p2p:         p,
					initialSync: &mockSync.Sync{},
					clock:       startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
					beaconDB:    db,
					stateGen:    sg,
				},
			}
			s.newExecutionPayloadEnvelopeVerifier = tt.verifier

			blk := random.SignedBeaconBlock(t)
			blkRoot, err := blk.Block.HashTreeRoot()
			require.NoError(t, err)
			msg := random.SignedExecutionPayloadEnvelope(t)
			msg.Message.BeaconBlockRoot = blkRoot[:]
			wblk, err := blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, wblk))
			st, err := util.NewBeaconStateEpbs()
			require.NoError(t, err)
			require.NoError(t, sg.SaveState(ctx, blkRoot, st))
			buf := new(bytes.Buffer)
			_, err = p.Encoding().EncodeGossip(buf, msg)
			require.NoError(t, err)

			topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
			digest, err := s.currentForkDigest()
			require.NoError(t, err)
			topic = s.addDigestToTopic(topic, digest)

			result, err := s.validateExecutionPayloadEnvelope(ctx, "", &pubsub.Message{
				Message: &pb.Message{
					Data:  buf.Bytes(),
					Topic: &topic,
				}})

			require.ErrorContains(t, tt.error.Error(), err)
			require.Equal(t, result, tt.result)
		})
	}
}

func TestValidateExecutionPayloadEnvelope_Accept(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	db := testDB.SetupDB(t)
	fcs := doublylinkedtree.New()
	sg := stategen.New(db, fcs)
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
	s := &Service{
		payloadEnvelopeCache: &sync.Map{},
		cfg: &config{
			chain:       chainService,
			p2p:         p,
			initialSync: &mockSync.Sync{},
			clock:       startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			beaconDB:    db,
			stateGen:    sg,
		},
	}
	s.newExecutionPayloadEnvelopeVerifier = func(e interfaces.ROSignedExecutionPayloadEnvelope, reqs []verification.Requirement) verification.ExecutionPayloadEnvelopeVerifier {
		return &verification.MockExecutionPayloadEnvelope{}
	}

	blk := random.SignedBeaconBlock(t)
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	msg := random.SignedExecutionPayloadEnvelope(t)
	msg.Message.BeaconBlockRoot = blkRoot[:]
	wblk, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wblk))
	st, err := util.NewBeaconStateEpbs()
	require.NoError(t, err)
	require.NoError(t, sg.SaveState(ctx, blkRoot, st))

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestToTopic(topic, digest)

	result, err := s.validateExecutionPayloadEnvelope(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}})
	require.NoError(t, err)
	require.Equal(t, result, pubsub.ValidationAccept)
}
