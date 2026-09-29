//go:build minimal

package validator

import (
	"testing"

	builderTest "github.com/OffchainLabs/prysm/v7/beacon-chain/builder/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
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
			Url:                 []byte(url),
			MaxExecutionPayment: primitives.Gwei(payment),
		}
	}
	newServer := func(builder *builderTest.MockBuilderService) *Server {
		return &Server{BlockBuilder: builder, SyncChecker: &mockSync.Sync{IsSyncing: false}}
	}
	req := &ethpb.SubmitBuilderPreferencesRequest{
		Entries: []*ethpb.BuilderPreferencesEntry{entry("http://builder", 1000)},
	}

	t.Run("forwards on success", func(t *testing.T) {
		vs := newServer(&builderTest.MockBuilderService{HasConfigured: true})
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.NoError(t, err)
	})

	t.Run("empty request errors", func(t *testing.T) {
		vs := newServer(&builderTest.MockBuilderService{HasConfigured: true})
		_, err := vs.SubmitBuilderPreferences(t.Context(), &ethpb.SubmitBuilderPreferencesRequest{})
		require.ErrorContains(t, "request is empty", err)
	})

	t.Run("syncing node errors", func(t *testing.T) {
		vs := &Server{
			BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true},
			SyncChecker:  &mockSync.Sync{IsSyncing: true},
		}
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.ErrorContains(t, "Syncing to latest head", err)
	})

	t.Run("entry without url is dropped, rest of the batch still submits", func(t *testing.T) {
		builder := &builderTest.MockBuilderService{HasConfigured: true}
		vs := newServer(builder)
		_, err := vs.SubmitBuilderPreferences(t.Context(), &ethpb.SubmitBuilderPreferencesRequest{
			Entries: []*ethpb.BuilderPreferencesEntry{entry("", 5), entry("http://builder", 7)},
		})
		require.NoError(t, err)
		require.DeepEqual(t, []string{"http://builder"}, builder.SubmittedPreferenceUrls())
	})

	t.Run("succeeds without the builder endpoint flag", func(t *testing.T) {
		vs := newServer(&builderTest.MockBuilderService{HasConfigured: false})
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.NoError(t, err)
	})

	t.Run("nil block builder errors", func(t *testing.T) {
		vs := &Server{SyncChecker: &mockSync.Sync{IsSyncing: false}}
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.ErrorContains(t, "builder is not configured", err)
	})

	t.Run("partial builder failure does not fail the batch", func(t *testing.T) {
		builder := &builderTest.MockBuilderService{
			HasConfigured:              true,
			ErrSubmitBuilderPrefsByURL: map[string]error{"http://down": errors.New("boom")},
		}
		vs := newServer(builder)
		_, err := vs.SubmitBuilderPreferences(t.Context(), &ethpb.SubmitBuilderPreferencesRequest{
			Entries: []*ethpb.BuilderPreferencesEntry{entry("http://down", 5), entry("http://builder", 7)},
		})
		require.NoError(t, err)
		require.DeepEqual(t, []string{"http://builder"}, builder.SubmittedPreferenceUrls())
	})

	t.Run("errors when every submission fails", func(t *testing.T) {
		vs := newServer(&builderTest.MockBuilderService{HasConfigured: true, ErrSubmitBuilderPreferences: errors.New("boom")})
		_, err := vs.SubmitBuilderPreferences(t.Context(), req)
		require.ErrorContains(t, "could not submit builder preferences to any builder", err)
	})
}
