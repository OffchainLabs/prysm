package validator

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	mockChain "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
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
