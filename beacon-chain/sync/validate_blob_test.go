package sync

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	lruwrpr "github.com/OffchainLabs/prysm/v6/cache/lru"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
)

func TestValidateBlob_FromSelf(t *testing.T) {
	ctx := t.Context()
	p := p2ptest.NewTestP2P(t)
	s := &Service{cfg: &config{p2p: p}}
	result, err := s.validateBlob(ctx, s.cfg.p2p.PeerID(), nil)
	require.NoError(t, err)
	require.Equal(t, result, pubsub.ValidationAccept)
}

func TestValidateBlob_InitSync(t *testing.T) {
	ctx := t.Context()
	p := p2ptest.NewTestP2P(t)
	s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{IsSyncing: true}}}
	result, err := s.validateBlob(ctx, "", nil)
	require.NoError(t, err)
	require.Equal(t, result, pubsub.ValidationIgnore)
}

func TestValidateBlob_InvalidTopic(t *testing.T) {
	ctx := t.Context()
	p := p2ptest.NewTestP2P(t)
	s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{}}}
	result, err := s.validateBlob(ctx, "", &pubsub.Message{
		Message: &pb.Message{},
	})
	require.ErrorIs(t, p2p.ErrInvalidTopic, err)
	require.Equal(t, result, pubsub.ValidationReject)
}

func TestValidateBlob_InvalidMessageType(t *testing.T) {
	ctx := t.Context()
	p := p2ptest.NewTestP2P(t)
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
	s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}
	s.newBlobVerifier = testNewBlobVerifier()

	msg := util.NewBeaconBlock()
	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestToTopic(topic, digest)
	result, err := s.validateBlob(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}})
	require.ErrorIs(t, errWrongMessage, err)
	require.Equal(t, result, pubsub.ValidationReject)
}

func TestValidateBlob_AlreadySeenInCache(t *testing.T) {
	db := dbtest.SetupDB(t)
	ctx := t.Context()
	p := p2ptest.NewTestP2P(t)
	chainService := &mock.ChainService{Genesis: time.Now(), FinalizedCheckPoint: &eth.Checkpoint{}, DB: db}
	stateGen := stategen.New(db, doublylinkedtree.New())
	s := &Service{
		seenBlobCache: lruwrpr.New(10),
		cfg: &config{
			p2p:         p,
			initialSync: &mockSync.Sync{},
			chain:       chainService,
			stateGen:    stateGen,
			clock:       startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}
	s.newBlobVerifier = testNewBlobVerifier()

	beaconState, _ := util.DeterministicGenesisState(t, 100)

	parent := util.NewBeaconBlock()
	parentBb, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)
	parentRoot, err := parentBb.Block().HashTreeRoot()
	require.NoError(t, err)

	bb := util.NewBeaconBlock()
	bb.Block.Slot = 1
	bb.Block.ParentRoot = parentRoot[:]
	bb.Block.ProposerIndex = 19026
	signedBb, err := blocks.NewSignedBeaconBlock(bb)
	require.NoError(t, err)

	require.NoError(t, db.SaveBlock(ctx, parentBb))
	require.NoError(t, db.SaveBlock(ctx, signedBb))
	r, err := signedBb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, r))

	header, err := signedBb.Header()
	require.NoError(t, err)
	sc := util.GenerateTestDenebBlobSidecar(t, r, header, 0, make([]byte, 48), make([][]byte, 0))
	b := sc.BlobSidecar

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, b)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(b)]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestAndIndexToTopic(topic, digest, 0)

	s.setSeenBlobIndex(sc.Slot(), sc.SignedBlockHeader.Header.ProposerIndex, 0)
	result, err := s.validateBlob(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}})
	require.NoError(t, err)
	require.Equal(t, result, pubsub.ValidationIgnore)
}

func TestValidateBlob_InvalidTopicIndex(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	ctx := t.Context()
	p := p2ptest.NewTestP2P(t)
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
	s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, params.BeaconConfig().GenesisValidatorsRoot)}}
	s.newBlobVerifier = testNewBlobVerifier()

	_, scs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, chainService.CurrentSlot()+1, 1)
	msg := scs[0].BlobSidecar
	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestAndIndexToTopic(topic, digest, 1)
	result, err := s.validateBlob(ctx, "", &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		}})
	require.ErrorContains(t, "blob_sidecar_1", err)
	require.Equal(t, result, pubsub.ValidationReject)
}

func TestValidateBlob_ErrorPathsWithMock(t *testing.T) {
	tests := []struct {
		name     string
		error    error
		verifier verification.NewBlobVerifier
		result   pubsub.ValidationResult
	}{
		{
			error: errors.New("blob index out of bound"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrBlobIndexInBounds: errors.New("blob index out of bound")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("slot too early"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrSlotTooEarly: errors.New("slot too early")}
			},
			result: pubsub.ValidationIgnore,
		},
		{
			error: errors.New("slot above finalized"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrSlotAboveFinalized: errors.New("slot above finalized")}
			},
			result: pubsub.ValidationIgnore,
		},
		{
			error: errors.New("valid proposer signature"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrValidProposerSignature: errors.New("valid proposer signature")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("sidecar parent seen"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrSidecarParentSeen: errors.New("sidecar parent seen")}
			},
			result: pubsub.ValidationIgnore,
		},
		{
			error: errors.New("sidecar parent valid"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrSidecarParentValid: errors.New("sidecar parent valid")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("sidecar parent slot lower"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrSidecarParentSlotLower: errors.New("sidecar parent slot lower")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("descends from finalized"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrSidecarDescendsFromFinalized: errors.New("descends from finalized")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("inclusion proven"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrSidecarInclusionProven: errors.New("inclusion proven")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("kzg proof verified"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrSidecarKzgProofVerified: errors.New("kzg proof verified")}
			},
			result: pubsub.ValidationReject,
		},
		{
			error: errors.New("sidecar proposer expected"),
			verifier: func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
				return &verification.MockBlobVerifier{ErrSidecarProposerExpected: errors.New("sidecar proposer expected")}
			},
			result: pubsub.ValidationReject,
		},
	}
	for _, tt := range tests {
		t.Run(tt.error.Error(), func(t *testing.T) {
			ctx := t.Context()
			p := p2ptest.NewTestP2P(t)
			chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)}
			s := &Service{
				seenBlobCache:     lruwrpr.New(10),
				seenPendingBlocks: make(map[[32]byte]bool),
				cfg:               &config{chain: chainService, p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)}}
			s.newBlobVerifier = tt.verifier

			_, scs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, chainService.CurrentSlot()+1, 1)
			msg := scs[0].BlobSidecar
			buf := new(bytes.Buffer)
			_, err := p.Encoding().EncodeGossip(buf, msg)
			require.NoError(t, err)

			topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
			digest, err := s.currentForkDigest()
			require.NoError(t, err)
			topic = s.addDigestAndIndexToTopic(topic, digest, 0)
			result, err := s.validateBlob(ctx, "", &pubsub.Message{
				Message: &pb.Message{
					Data:  buf.Bytes(),
					Topic: &topic,
				}})
			require.ErrorContains(t, tt.error.Error(), err)
			require.Equal(t, result, tt.result)
		})
	}
}

func testNewBlobVerifier() verification.NewBlobVerifier {
	return func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
		return &verification.MockBlobVerifier{}
	}
}
