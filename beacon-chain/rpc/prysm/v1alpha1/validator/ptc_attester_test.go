package validator

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
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

func TestServer_GetPayloadAttestationData(t *testing.T) {
	ctx := context.Background()
	t.Run("Not current slot", func(t *testing.T) {
		s := &Server{
			TimeFetcher: &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)},
		}
		_, err := s.GetPayloadAttestationData(ctx, &ethpb.GetPayloadAttestationDataRequest{Slot: 2})
		require.ErrorContains(t, "Payload attestation request slot 2 != current slot 1", err)
	})

	t.Run("Last received block is not from current slot", func(t *testing.T) {
		s := &Server{
			TimeFetcher:       &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(2*params.BeaconConfig().SecondsPerSlot), 0)},
			ForkchoiceFetcher: &mock.ChainService{HighestReceivedSlot: 1},
		}
		_, err := s.GetPayloadAttestationData(ctx, &ethpb.GetPayloadAttestationDataRequest{Slot: 2})
		require.ErrorContains(t, "Did not receive current slot 2 block ", err)
	})

	t.Run("Payload is absent", func(t *testing.T) {
		slot := primitives.Slot(2)
		root := [32]byte{1}
		s := &Server{
			TimeFetcher:       &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(2*params.BeaconConfig().SecondsPerSlot), 0)},
			ForkchoiceFetcher: &mock.ChainService{HighestReceivedSlot: slot, HighestReceivedRoot: root, PayloadStatus: primitives.PAYLOAD_ABSENT},
		}
		d, err := s.GetPayloadAttestationData(ctx, &ethpb.GetPayloadAttestationDataRequest{Slot: slot})
		require.NoError(t, err)
		require.DeepEqual(t, root[:], d.BeaconBlockRoot)
		require.Equal(t, slot, d.Slot)
		require.Equal(t, primitives.PAYLOAD_ABSENT, d.PayloadStatus)
	})

	t.Run("Payload is present", func(t *testing.T) {
		slot := primitives.Slot(2)
		root := [32]byte{1}
		s := &Server{
			TimeFetcher:       &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(2*params.BeaconConfig().SecondsPerSlot), 0)},
			ForkchoiceFetcher: &mock.ChainService{HighestReceivedSlot: slot, HighestReceivedRoot: root, PayloadStatus: primitives.PAYLOAD_PRESENT},
		}
		d, err := s.GetPayloadAttestationData(ctx, &ethpb.GetPayloadAttestationDataRequest{Slot: slot})
		require.NoError(t, err)
		require.DeepEqual(t, root[:], d.BeaconBlockRoot)
		require.Equal(t, slot, d.Slot)
		require.Equal(t, primitives.PAYLOAD_PRESENT, d.PayloadStatus)
	})

	t.Run("Payload is withheld", func(t *testing.T) {
		slot := primitives.Slot(2)
		root := [32]byte{1}
		s := &Server{
			TimeFetcher:       &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(2*params.BeaconConfig().SecondsPerSlot), 0)},
			ForkchoiceFetcher: &mock.ChainService{HighestReceivedSlot: slot, HighestReceivedRoot: root, PayloadStatus: primitives.PAYLOAD_WITHHELD},
		}
		d, err := s.GetPayloadAttestationData(ctx, &ethpb.GetPayloadAttestationDataRequest{Slot: slot})
		require.NoError(t, err)
		require.DeepEqual(t, root[:], d.BeaconBlockRoot)
		require.Equal(t, slot, d.Slot)
		require.Equal(t, primitives.PAYLOAD_WITHHELD, d.PayloadStatus)
	})
}
