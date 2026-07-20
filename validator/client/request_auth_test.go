package client

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/protobuf/proto"
)

func cachedAuth(data []byte, slot primitives.Slot) *ethpb.SignedRequestAuthV1 {
	return &ethpb.SignedRequestAuthV1{Message: &ethpb.RequestAuthV1{Data: data, Slot: slot}}
}

func settingsWithBuilders(pk [fieldparams.BLSPubkeyLength]byte, entries ...*proposer.BuilderEntry) *proposer.Settings {
	return &proposer.Settings{
		Version: proposer.SchemaV2,
		ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
			pk: {BuilderConfig: &proposer.BuilderConfig{Enabled: proto.Bool(true), Builders: entries}},
		},
	}
}

// A cached auth signed over rotated-away auth_data must never be attached to a proposal.
func TestBuildBuilderPreferencesForSlot_StaleAuthNotServedAfterRotation(t *testing.T) {
	pk := [fieldparams.BLSPubkeyLength]byte{1}
	slot := primitives.Slot(10)
	url := "https://builder.example"

	v := validator{
		proposerSettings: settingsWithBuilders(pk, &proposer.BuilderEntry{URL: url, AuthData: []byte("B")}),
		signedRequestAuths: map[requestAuthKey]*ethpb.SignedRequestAuthV1{
			{pk: pk, slot: slot, relay: url, authData: "A"}: cachedAuth([]byte("A"), slot),
		},
	}

	require.Equal(t, 0, len(v.buildBuilderPreferencesForSlot(pk, slot)))

	v.signedRequestAuths[requestAuthKey{pk: pk, slot: slot, relay: url, authData: "B"}] = cachedAuth([]byte("B"), slot)
	prefs := v.buildBuilderPreferencesForSlot(pk, slot)
	require.Equal(t, 1, len(prefs))
	require.Equal(t, url, prefs[0].Url)
	require.DeepEqual(t, []byte("B"), prefs[0].Request.Auth.Message.Data)
}

// Duplicate URLs with distinct auth_data must each pair with their own signed auth.
func TestBuildBuilderPreferencesForSlot_DuplicateURLDistinctAuthData(t *testing.T) {
	pk := [fieldparams.BLSPubkeyLength]byte{2}
	slot := primitives.Slot(7)
	url := "https://builder.example"

	v := validator{
		proposerSettings: settingsWithBuilders(pk,
			&proposer.BuilderEntry{URL: url, AuthData: []byte("A")},
			&proposer.BuilderEntry{URL: url, AuthData: []byte("B")},
		),
		signedRequestAuths: map[requestAuthKey]*ethpb.SignedRequestAuthV1{
			{pk: pk, slot: slot, relay: url, authData: "A"}: cachedAuth([]byte("A"), slot),
			{pk: pk, slot: slot, relay: url, authData: "B"}: cachedAuth([]byte("B"), slot),
		},
	}

	prefs := v.buildBuilderPreferencesForSlot(pk, slot)
	require.Equal(t, 2, len(prefs))
	require.DeepEqual(t, []byte("A"), prefs[0].Request.Auth.Message.Data)
	require.DeepEqual(t, []byte("B"), prefs[1].Request.Auth.Message.Data)
}

// Drives the real sign path (as warmBuilderRequestAuthsForDuties does) so a key-shape
// mismatch between signRequestAuthCached and signedRequestAuthFor cannot go unnoticed.
func TestSignRequestAuthCached_RotationRoundTrip(t *testing.T) {
	kp := randKeypair(t)
	km := newMockKeymanager(t, kp)
	ctx := t.Context()
	slot := primitives.Slot(20)
	url := "https://builder.example"

	v := validator{proposerSettings: settingsWithBuilders(kp.pub, &proposer.BuilderEntry{URL: url, AuthData: []byte("A")})}
	warm := func() {
		for _, tgt := range v.builderTargetsForKey(kp.pub) {
			_, err := v.signRequestAuthCached(ctx, km, kp.pub, tgt.url, tgt.authData, slot)
			require.NoError(t, err)
		}
	}

	warm()
	prefs := v.buildBuilderPreferencesForSlot(kp.pub, slot)
	require.Equal(t, 1, len(prefs))
	require.DeepEqual(t, []byte("A"), prefs[0].Request.Auth.Message.Data)

	// Rotate auth_data; the cached A-auth must not be served, and re-warming must re-sign.
	v.proposerSettings = settingsWithBuilders(kp.pub, &proposer.BuilderEntry{URL: url, AuthData: []byte("B")})
	require.Equal(t, 0, len(v.buildBuilderPreferencesForSlot(kp.pub, slot)))

	warm()
	prefs = v.buildBuilderPreferencesForSlot(kp.pub, slot)
	require.Equal(t, 1, len(prefs))
	require.DeepEqual(t, []byte("B"), prefs[0].Request.Auth.Message.Data)
	require.Equal(t, 2, len(v.signedRequestAuths))

	v.pruneSignedRequestAuths(slot + 1)
	require.Equal(t, 0, len(v.signedRequestAuths))
}

func TestSignedRequestAuthFor_KeyIncludesAuthData(t *testing.T) {
	pk := [fieldparams.BLSPubkeyLength]byte{3}
	slot := primitives.Slot(4)
	url := "https://builder.example"

	v := validator{
		signedRequestAuths: map[requestAuthKey]*ethpb.SignedRequestAuthV1{
			{pk: pk, slot: slot, relay: url, authData: "A"}: cachedAuth([]byte("A"), slot),
		},
	}

	_, ok := v.signedRequestAuthFor(pk, url, []byte("B"), slot)
	require.Equal(t, false, ok)
	got, ok := v.signedRequestAuthFor(pk, url, []byte("A"), slot)
	require.Equal(t, true, ok)
	require.DeepEqual(t, []byte("A"), got.Message.Data)
}
