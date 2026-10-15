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

	decoded := &SignedBeaconBlockHeze{}
	require.NoError(t, json.Unmarshal(encoded, decoded))
	got, err := decoded.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, blk, got)

	generic, err := decoded.ToGeneric()
	require.NoError(t, err)
	require.DeepEqual(t, blk, generic.GetHeze())
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
