package structs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestSignedBeaconBlockHezeRoundTrip(t *testing.T) {
	blk := util.NewBeaconBlockHeze()
	blk.Block.Slot = 7
	blk.Block.Body.SignedExecutionPayloadBid.Message.InclusionListBits = []byte{0x01, 0x80}

	jsonBlock, err := SignedBeaconBlockHezeFromConsensus(blk)
	require.NoError(t, err)
	encoded, err := json.Marshal(jsonBlock)
	require.NoError(t, err)
	require.Equal(t, true, strings.Contains(string(encoded), `"inclusion_list_bits":"0x0180"`))
	require.Equal(t, false, strings.Contains(string(encoded), "eth1_data"))

	decoded := &SignedBeaconBlockHeze{}
	require.NoError(t, json.Unmarshal(encoded, decoded))
	got, err := decoded.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, blk, got)

	generic, err := decoded.ToGeneric()
	require.NoError(t, err)
	require.DeepEqual(t, blk, generic.GetHeze())
}

func TestBeaconBlockBodyHeze_ToConsensus_NilFields(t *testing.T) {
	tests := []struct {
		name  string
		field string
		clear func(b *BeaconBlockBodyHeze)
	}{
		{name: "nil SyncAggregate", field: "SyncAggregate", clear: func(b *BeaconBlockBodyHeze) { b.SyncAggregate = nil }},
		{name: "nil SignedExecutionPayloadBid", field: "SignedExecutionPayloadBid", clear: func(b *BeaconBlockBodyHeze) { b.SignedExecutionPayloadBid = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blk, err := SignedBeaconBlockHezeFromConsensus(util.NewBeaconBlockHeze())
			require.NoError(t, err)
			body := blk.Message.Body
			tt.clear(body)
			_, err = body.ToConsensus()
			require.ErrorContains(t, "could not decode "+tt.field+": "+errNilValue.Error(), err)
		})
	}
}

func TestExecutionPayloadBidHezeToConsensus_BadInclusionListBits(t *testing.T) {
	bid := ExecutionPayloadBidHezeFromConsensus(util.NewBeaconBlockHeze().Block.Body.SignedExecutionPayloadBid.Message)
	for _, bits := range []string{"0x01", "0x010203", "zz"} {
		t.Run(bits, func(t *testing.T) {
			bid.InclusionListBits = bits
			_, err := bid.ToConsensus()
			require.ErrorContains(t, "InclusionListBits", err)
		})
	}
}
