package eth

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBuilderPreferencesSSZRoundTrip(t *testing.T) {
	pubkey := make([]byte, 48)
	pubkey[0] = 0xAB
	boost := uint64(200)
	minBid := primitives.Gwei(500)
	prefs := []*BuilderPreferenceV1{
		{
			Url: "https://a.example",
			Request: &BuilderPreferencesRequestV1{
				Preferences: &BuilderPreferencesV1{MaxExecutionPayment: 1000},
				Auth:        &SignedRequestAuthV1{Message: &RequestAuthV1{Data: []byte("auth"), Slot: 7}, Signature: make([]byte, 96)},
			},
			MinBid:             &minBid,
			BuilderBoostFactor: &boost,
			Pubkey:             pubkey,
		},
		{
			// Optionals absent: min_bid, boost, and pubkey resolve to sentinels.
			Url: "https://b.example",
			Request: &BuilderPreferencesRequestV1{
				Preferences: &BuilderPreferencesV1{},
				Auth:        &SignedRequestAuthV1{Message: &RequestAuthV1{}, Signature: make([]byte, 96)},
			},
		},
	}

	ssz, err := BuilderPreferencesToSSZ(prefs).MarshalSSZ()
	require.NoError(t, err)
	list := &ProduceBuilderEntryListV1{}
	require.NoError(t, list.UnmarshalSSZ(ssz))
	got := BuilderPreferencesFromSSZ(list)

	require.Equal(t, 2, len(got))
	require.Equal(t, "https://a.example", got[0].Url)
	require.Equal(t, primitives.Gwei(500), got[0].GetMinBid())
	require.Equal(t, uint64(200), got[0].GetBuilderBoostFactor())
	require.DeepEqual(t, pubkey, got[0].Pubkey)
	require.Equal(t, primitives.Gwei(1000), got[0].Request.Preferences.GetMaxExecutionPayment())
	require.DeepEqual(t, []byte("auth"), got[0].Request.Auth.Message.Data)

	require.Equal(t, primitives.Gwei(0), got[1].GetMinBid())
	require.Equal(t, uint64(100), got[1].GetBuilderBoostFactor())
	require.IsNil(t, got[1].Pubkey)
}
