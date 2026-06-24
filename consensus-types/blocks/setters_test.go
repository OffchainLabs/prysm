package blocks

import (
	"testing"

	bitfield "github.com/OffchainLabs/go-bitfield"
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSignedBeaconBlock_SetPayloadAttestations(t *testing.T) {
	t.Run("rejects pre-Gloas versions", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Fulu)
		payload := []*eth.PayloadAttestation{{}}

		err := sb.SetPayloadAttestations(payload)

		require.ErrorIs(t, err, consensus_types.ErrUnsupportedField)
		require.IsNil(t, sb.block.body.payloadAttestations)
	})

	t.Run("sets payload attestations for Gloas", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Gloas)
		payload := []*eth.PayloadAttestation{
			{
				AggregationBits: bitfield.NewBitvector512(),
				Data: &eth.PayloadAttestationData{
					BeaconBlockRoot:   []byte{0x01, 0x02},
					PayloadPresent:    true,
					BlobDataAvailable: true,
				},
				Signature: []byte{0x03},
			},
		}

		err := sb.SetPayloadAttestations(payload)

		require.NoError(t, err)
		require.DeepEqual(t, payload, sb.block.body.payloadAttestations)
	})
}

func TestSignedBeaconBlock_SetSignedExecutionPayloadBid(t *testing.T) {
	t.Run("rejects pre-Gloas versions", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Fulu)
		payloadBid := &eth.SignedExecutionPayloadBid{}

		err := sb.SetSignedExecutionPayloadBid(payloadBid)

		require.ErrorIs(t, err, consensus_types.ErrUnsupportedField)
		require.IsNil(t, sb.block.body.signedExecutionPayloadBid)
	})

	t.Run("sets signed execution payload bid for Gloas", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Gloas)
		payloadBid := &eth.SignedExecutionPayloadBid{
			Message: &eth.ExecutionPayloadBid{
				ParentBlockHash: []byte{0xaa},
				BlockHash:       []byte{0xbb},
				FeeRecipient:    []byte{0xcc},
			},
			Signature: []byte{0xdd},
		}

		err := sb.SetSignedExecutionPayloadBid(payloadBid)

		require.NoError(t, err)
		require.Equal(t, payloadBid, sb.block.body.signedExecutionPayloadBid)
	})
}

func TestSignedBeaconBlock_SetExecution(t *testing.T) {
	t.Run("rejects Gloas version", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Gloas)
		payload := &enginev1.ExecutionPayload{}
		wrapped, err := WrappedExecutionPayload(payload)
		require.NoError(t, err)

		err = sb.SetExecution(wrapped)
		require.ErrorIs(t, err, consensus_types.ErrUnsupportedField)
	})
}

func TestSignedBeaconBlock_SetExecutionRequests(t *testing.T) {
	t.Run("rejects Gloas version", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Gloas)
		requests := &enginev1.ExecutionRequests{}

		err := sb.SetExecutionRequests(requests)
		require.ErrorIs(t, err, consensus_types.ErrUnsupportedField)
	})
}

func newTestSignedBeaconBlock(ver int) *SignedBeaconBlock {
	return &SignedBeaconBlock{
		version: ver,
		block: &BeaconBlock{
			version: ver,
			body: &BeaconBlockBody{
				version: ver,
			},
		},
	}
}
