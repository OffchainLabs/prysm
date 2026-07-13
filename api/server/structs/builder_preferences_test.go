package structs

import (
	"bytes"
	"testing"

	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBuilderPreferencesRequestV1_RoundTrip(t *testing.T) {
	original := &eth.BuilderPreferencesRequestV1{
		Preferences: &eth.BuilderPreferencesV1{MaxExecutionPayment: 1_000_000},
		Auth: &eth.SignedRequestAuthV1{
			Message: &eth.RequestAuthV1{
				Data: []byte("https://builder.example.com"),
				Slot: 32,
			},
			Signature: bytes.Repeat([]byte{0x01}, 96),
		},
	}

	got, err := BuilderPreferencesRequestV1FromConsensus(original).ToConsensus()
	require.NoError(t, err)
	assert.Equal(t, uint64(original.Preferences.MaxExecutionPayment), uint64(got.Preferences.MaxExecutionPayment))
	assert.Equal(t, uint64(original.Auth.Message.Slot), uint64(got.Auth.Message.Slot))
	assert.DeepEqual(t, original.Auth.Message.Data, got.Auth.Message.Data)
	assert.DeepEqual(t, original.Auth.Signature, got.Auth.Signature)
}

func TestRequestAuthV1_ToConsensus_DataTooLarge(t *testing.T) {
	a := &RequestAuthV1{
		Data: "0x" + string(bytes.Repeat([]byte("61"), maxRequestAuthDataSize+1)),
		Slot: "1",
	}
	_, err := a.ToConsensus()
	assert.ErrorContains(t, "exceeds max", err)
}

func TestBuilderPreferencesRequestV1_ToConsensus_Nil(t *testing.T) {
	var r *BuilderPreferencesRequestV1
	_, err := r.ToConsensus()
	assert.ErrorContains(t, errNilValue.Error(), err)
}
