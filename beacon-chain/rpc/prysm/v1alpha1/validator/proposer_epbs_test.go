package validator

import (
	"context"
	"testing"

	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	dbutil "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/pkg/errors"
)

func TestServer_SubmitSignedExecutionPayloadEnvelope(t *testing.T) {
	env := &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			Payload:            &enginev1.ExecutionPayloadDeneb{},
			BeaconBlockRoot:    make([]byte, 32),
			BlobKzgCommitments: [][]byte{},
			BeaconStateRoot:    make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}
	t.Run("Happy case", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateEpbs(t, 1)
		s := &Server{
			P2P:                      p2ptest.NewTestP2P(t),
			ExecutionPayloadReceiver: &mockChain.ChainService{State: st},
		}
		_, err := s.SubmitSignedExecutionPayloadEnvelope(context.Background(), env)
		require.NoError(t, err)
	})

	t.Run("Receive failed", func(t *testing.T) {
		s := &Server{
			P2P:                      p2ptest.NewTestP2P(t),
			ExecutionPayloadReceiver: &mockChain.ChainService{ReceiveBlockMockErr: errors.New("receive failed")},
		}
		_, err := s.SubmitSignedExecutionPayloadEnvelope(context.Background(), env)
		require.ErrorContains(t, "failed to receive execution payload envelope: receive failed", err)
	})
}

func TestServer_SubmitSignedExecutionPayloadHeader(t *testing.T) {
	st, _ := util.DeterministicGenesisStateEpbs(t, 1)
	h := &enginev1.SignedExecutionPayloadHeader{
		Message: &enginev1.ExecutionPayloadHeaderEPBS{
			Slot: 1,
		},
	}
	slot := primitives.Slot(1)
	server := &Server{
		TimeFetcher: &mockChain.ChainService{Slot: &slot},
		HeadFetcher: &mockChain.ChainService{State: st},
		P2P:         p2ptest.NewTestP2P(t),
	}

	t.Run("Happy case", func(t *testing.T) {
		h.Message.BuilderIndex = 1
		_, err := server.SubmitSignedExecutionPayloadHeader(context.Background(), h)
		require.NoError(t, err)
		require.DeepEqual(t, server.signedExecutionPayloadHeader, h)
	})

	t.Run("Incorrect slot", func(t *testing.T) {
		h.Message.Slot = 3
		_, err := server.SubmitSignedExecutionPayloadHeader(context.Background(), h)
		require.ErrorContains(t, "invalid slot: current slot 1, got 3", err)
	})
}

func TestProposer_ComputePostPayloadStateRoot(t *testing.T) {
	db := dbutil.SetupDB(t)
	ctx := context.Background()

	proposerServer := &Server{
		ChainStartFetcher: &mockExecution.Chain{},
		Eth1InfoFetcher:   &mockExecution.Chain{},
		Eth1BlockFetcher:  &mockExecution.Chain{},
		StateGen:          stategen.New(db, doublylinkedtree.New()),
	}

	bh := [32]byte{'h'}
	expectedStateRoot := [32]byte{0x36, 0xbd, 0xd4, 0xd4, 0x74, 0x94, 0x8e, 0x3b, 0xc2, 0x70, 0xd9, 0xf1, 0x62, 0x1b, 0x63, 0x1, 0x31, 0x29, 0x41, 0xd2, 0xbd, 0x6c, 0x8, 0xa7, 0x8a, 0x57, 0xfe, 0x29, 0xb, 0x75, 0xef, 0xb}
	p := &enginev1.ExecutionPayloadEnvelope{
		BeaconBlockRoot:    make([]byte, 32),
		Payload:            &enginev1.ExecutionPayloadDeneb{},
		ExecutionRequests:  &enginev1.ExecutionRequests{},
		BlobKzgCommitments: make([][]byte, 0),
		BeaconStateRoot:    expectedStateRoot[:],
	}
	p.Payload.BlockHash = bh[:]
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)

	st, _ := util.DeterministicGenesisStateEpbs(t, 64)
	blockHeader := st.LatestBlockHeader()
	sr, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	blockHeader.StateRoot = sr[:]
	blockHeaderRoot, err := blockHeader.HashTreeRoot()
	require.NoError(t, err)
	p.BeaconBlockRoot = blockHeaderRoot[:]

	require.NoError(t, db.SaveState(ctx, st, e.BeaconBlockRoot()))
	stateRoot, err := proposerServer.computePostPayloadStateRoot(ctx, e)
	require.NoError(t, err)
	require.DeepEqual(t, expectedStateRoot[:], stateRoot)
}
