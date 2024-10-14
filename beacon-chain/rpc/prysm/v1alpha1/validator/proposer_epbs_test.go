package validator

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	mockChain "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	dbutil "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	mockExecution "github.com/prysmaticlabs/prysm/v5/beacon-chain/execution/testing"
	doublylinkedtree "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/doubly-linked-tree"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stategen"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestServer_SubmitSignedExecutionPayloadEnvelope(t *testing.T) {
	env := &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			Payload:            &enginev1.ExecutionPayloadElectra{},
			BeaconBlockRoot:    make([]byte, 32),
			BlobKzgCommitments: [][]byte{},
			StateRoot:          make([]byte, 32),
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
	root := [32]byte{'r'}
	expectedStateRoot := [32]byte{22, 85, 188, 95, 44, 156, 240, 10, 30, 106, 216, 244, 24, 39, 130, 196, 151, 118, 200, 94, 28, 42, 13, 170, 109, 206, 33, 83, 97, 154, 53, 251}
	p := &enginev1.ExecutionPayloadEnvelope{
		Payload:            &enginev1.ExecutionPayloadElectra{},
		BeaconBlockRoot:    root[:],
		BlobKzgCommitments: make([][]byte, 0),
		StateRoot:          expectedStateRoot[:],
	}
	p.Payload.BlockHash = bh[:]
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)

	st, _ := util.DeterministicGenesisStateEpbs(t, 64)
	require.NoError(t, db.SaveState(ctx, st, e.BeaconBlockRoot()))
	stateRoot, err := proposerServer.computePostPayloadStateRoot(ctx, e)
	require.NoError(t, err)
	require.DeepEqual(t, expectedStateRoot[:], stateRoot)
}
