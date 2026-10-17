package structs

import (
	"encoding/json"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestSignedBeaconBlockHezeRoundTrip(t *testing.T) {
	blk := util.NewBeaconBlockHeze()
	blk.Block.Slot = 7

	jsonBlock, err := SignedBeaconBlockHezeFromConsensus(blk)
	require.NoError(t, err)
	encoded, err := json.Marshal(jsonBlock)
	require.NoError(t, err)

	decoded := &SignedBeaconBlockHeze{}
	require.NoError(t, json.Unmarshal(encoded, decoded))
	got, err := decoded.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, blk, got)

	generic, err := decoded.ToGeneric()
	require.NoError(t, err)
	require.DeepEqual(t, blk, generic.GetHeze())
}
