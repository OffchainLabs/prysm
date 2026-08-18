//go:build minimal

package validator

import (
	"testing"

	builderTest "github.com/OffchainLabs/prysm/v7/beacon-chain/builder/testing"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

func TestServer_SubmitBuilderPreferences(t *testing.T) {
	pubkey := bytesutil.ToBytes48([]byte{1, 2, 3})
	entry := func(url string, payment uint64) *ethpb.BuilderPreferencesEntry {
		return &ethpb.BuilderPreferencesEntry{
			ProposerPubkey:      pubkey[:],
			Url:                 url,
			MaxExecutionPayment: primitives.Gwei(payment),
		}
	}
	req := &ethpb.SubmitBuilderPreferencesRequest{
		Entries: []*ethpb.BuilderPreferencesEntry{entry("http://builder", 1000)},
	}

	t.Run("stores max execution payment on success", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.NoError(t, err)
		v, ok := vs.maxExecutionPayments.Load(pubkey)
		require.Equal(t, true, ok)
		require.Equal(t, uint64(1000), v.(uint64))
	})

	t.Run("empty request errors", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), &ethpb.SubmitBuilderPreferencesRequest{})
		require.ErrorContains(t, "request is empty", err)
	})

	t.Run("entry without url is skipped, rest of the batch submits", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), &ethpb.SubmitBuilderPreferencesRequest{
			Entries: []*ethpb.BuilderPreferencesEntry{entry("", 5), entry("http://builder", 7)},
		})
		require.NoError(t, err)
		v, ok := vs.maxExecutionPayments.Load(pubkey)
		require.Equal(t, true, ok)
		require.Equal(t, uint64(7), v.(uint64))
	})

	t.Run("succeeds without the builder endpoint flag", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: false}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.NoError(t, err)
		v, ok := vs.maxExecutionPayments.Load(pubkey)
		require.Equal(t, true, ok)
		require.Equal(t, uint64(1000), v.(uint64))
	})

	t.Run("nil block builder errors", func(t *testing.T) {
		vs := &Server{}
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.ErrorContains(t, "builder is not configured", err)
	})

	t.Run("does not store when builder submission fails", func(t *testing.T) {
		vs := &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true, ErrSubmitBuilderPreferences: errors.New("boom")}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.NoError(t, err)
		_, ok := vs.maxExecutionPayments.Load(pubkey)
		require.Equal(t, false, ok)
	})
}
