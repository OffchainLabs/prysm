package blocks

import (
	"testing"

	bitfield "github.com/OffchainLabs/go-bitfield"
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
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

// stubSignedBid is a foreign ROSignedExecutionPayloadBid implementation that the setter cannot unwrap.
type stubSignedBid struct{}

func (stubSignedBid) Bid() interfaces.ROExecutionPayloadBid { return nil }
func (stubSignedBid) Signature() [96]byte                   { return [96]byte{} }
func (stubSignedBid) SigningRoot([]byte) ([32]byte, error)  { return [32]byte{}, nil }
func (stubSignedBid) HashTreeRoot() ([32]byte, error)       { return [32]byte{}, nil }
func (stubSignedBid) IsNil() bool                           { return false }

func TestSignedBeaconBlock_SetSignedExecutionPayloadBid(t *testing.T) {
	gloasPb := hydrateBeaconBlockBodyGloas().SignedExecutionPayloadBid
	gloasBid, err := WrappedROSignedExecutionPayloadBid(gloasPb)
	require.NoError(t, err)
	hezePb := hydrateBeaconBlockBodyHeze().SignedExecutionPayloadBid
	hezeBid, err := WrappedROSignedExecutionPayloadBidHeze(hezePb)
	require.NoError(t, err)

	t.Run("rejects pre-Gloas versions", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Fulu)

		err := sb.SetSignedExecutionPayloadBid(gloasBid)

		require.ErrorIs(t, err, consensus_types.ErrUnsupportedField)
		require.IsNil(t, sb.block.body.signedExecutionPayloadBid)
	})

	t.Run("rejects nil bid", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Gloas)
		require.ErrorIs(t, sb.SetSignedExecutionPayloadBid(nil), consensus_types.ErrNilObjectWrapped)
	})

	t.Run("rejects foreign bid implementation", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Gloas)
		err := sb.SetSignedExecutionPayloadBid(stubSignedBid{})
		require.ErrorContains(t, "unsupported signed execution payload bid type", err)
		require.IsNil(t, sb.block.body.signedExecutionPayloadBid)
	})

	t.Run("sets signed execution payload bid for Gloas", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Gloas)

		require.NoError(t, sb.SetSignedExecutionPayloadBid(gloasBid))
		require.Equal(t, gloasPb, sb.block.body.signedExecutionPayloadBid)
		require.ErrorIs(t, sb.SetSignedExecutionPayloadBid(hezeBid), errIncorrectBlockVersion)
	})

	t.Run("sets signed execution payload bid for Heze", func(t *testing.T) {
		sb := newTestSignedBeaconBlock(version.Heze)

		require.NoError(t, sb.SetSignedExecutionPayloadBid(hezeBid))
		require.Equal(t, hezePb, sb.block.body.signedExecutionPayloadBidHeze)
		require.IsNil(t, sb.block.body.signedExecutionPayloadBid)
		require.ErrorIs(t, sb.SetSignedExecutionPayloadBid(gloasBid), errIncorrectBlockVersion)
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
