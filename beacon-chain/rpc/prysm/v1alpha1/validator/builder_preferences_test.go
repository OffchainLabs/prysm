package validator

import (
	"testing"

	builderTest "github.com/OffchainLabs/prysm/v7/beacon-chain/builder/testing"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

func aotPrefReq(url string) *ethpb.SubmitBuilderPreferencesRequest {
	return &ethpb.SubmitBuilderPreferencesRequest{
		ValidatorPubkey: make([]byte, 48),
		Url:             url,
		Request: &ethpb.BuilderPreferencesRequestV1{
			Preferences: &ethpb.BuilderPreferencesV1{MaxExecutionPayment: 1000},
			Auth:        &ethpb.SignedRequestAuthV1{Message: &ethpb.RequestAuthV1{Data: []byte("opaque")}},
		},
	}
}

func TestSubmitBuilderPreferences(t *testing.T) {
	t.Run("forwards to the builder service", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), aotPrefReq("http://b"))
		require.NoError(t, err)
	})
	t.Run("propagates builder errors", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{ErrSubmitBuilderPreferences: errors.New("boom")}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), aotPrefReq("http://b"))
		require.ErrorContains(t, "could not submit builder preferences", err)
	})
	t.Run("rejects missing url", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), aotPrefReq(""))
		require.ErrorContains(t, "malformed builder url", err)
	})
	t.Run("rejects malformed url", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), aotPrefReq("ftp://nohost"))
		require.ErrorContains(t, "malformed builder url", err)
	})
	t.Run("rejects empty request", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), &ethpb.SubmitBuilderPreferencesRequest{Url: "http://b"})
		require.ErrorContains(t, "request is empty", err)
	})
	t.Run("requires a builder", func(t *testing.T) {
		vs := &Server{}
		_, err := vs.SubmitBuilderPreferences(t.Context(), aotPrefReq("http://b"))
		require.ErrorContains(t, "builder is not configured", err)
	})
}
