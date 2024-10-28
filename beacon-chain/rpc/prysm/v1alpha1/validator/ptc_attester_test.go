package validator

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestServer_SubmitPayloadAttestation(t *testing.T) {
	ctx := context.Background()
	t.Run("Error", func(t *testing.T) {
		s := &Server{
			P2P:                        p2ptest.NewTestP2P(t),
			PayloadAttestationReceiver: &mock.ChainService{ReceivePayloadAttestationMessageErr: errors.New("error")},
		}
		_, err := s.SubmitPayloadAttestation(ctx, &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				Slot: 1,
			},
		})
		require.ErrorContains(t, "error", err)
	})

	t.Run("Happy case", func(t *testing.T) {
		s := &Server{
			P2P:                        p2ptest.NewTestP2P(t),
			PayloadAttestationReceiver: &mock.ChainService{},
		}
		_, err := s.SubmitPayloadAttestation(ctx, &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				Slot: 1,
			},
		})
		require.NoError(t, err)
	})
}
