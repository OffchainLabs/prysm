package validator

import (
	"testing"

	builderTest "github.com/OffchainLabs/prysm/v7/beacon-chain/builder/testing"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

func TestServer_SubmitBuilderPreferences(t *testing.T) {
	pubkey := bytesutil.ToBytes48([]byte{1, 2, 3})
	req := &ethpb.SubmitBuilderPreferencesRequest{
		ValidatorPubkey: pubkey[:],
		Request: &ethpb.BuilderPreferencesRequestV1{
			Preferences: &ethpb.BuilderPreferencesV1{MaxExecutionPayment: 1000},
		},
	}

	t.Run("stores max execution payment on success", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.NoError(t, err)
		v, ok := vs.maxExecutionPayments.Load(pubkey)
		require.Equal(t, true, ok)
		require.Equal(t, uint64(1000), v.(uint64))
	})

	t.Run("nil request errors", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), &ethpb.SubmitBuilderPreferencesRequest{ValidatorPubkey: pubkey[:]})
		require.ErrorContains(t, "request is empty", err)
	})

	t.Run("builder not configured errors", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: false}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.ErrorContains(t, "builder is not configured", err)
	})

	t.Run("does not store when builder submission fails", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true, ErrSubmitBuilderPreferences: errors.New("boom")}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.ErrorContains(t, "could not submit builder preferences", err)
		_, ok := vs.maxExecutionPayments.Load(pubkey)
		require.Equal(t, false, ok)
	})
}
