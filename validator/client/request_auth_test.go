package client

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	validatortypes "github.com/OffchainLabs/prysm/v7/consensus-types/validator"
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

// No two emitted entries may share a url; the first configured entry wins and
// pairs with the auth signed over its own auth_data.
func TestBuildBuilderPreferencesForSlot_DuplicateURLFirstEntryWins(t *testing.T) {
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
	require.Equal(t, 1, len(prefs))
	require.DeepEqual(t, []byte("A"), prefs[0].Request.Auth.Message.Data)
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

// Field-level inheritance vectors through target resolution (keymanager #87 discussion).
func TestBuilderTargetsForKey_FieldLevelInheritance(t *testing.T) {
	pk := [fieldparams.BLSPubkeyLength]byte{4}
	minBid := validatortypes.Uint64(5000000)
	zeroPayment := validatortypes.Uint64(0)
	defaultOption := &proposer.Option{
		BuilderConfig: &proposer.BuilderConfig{
			Enabled:  proto.Bool(true),
			Builders: []*proposer.BuilderEntry{{URL: "https://a"}, {URL: "https://b"}},
			MinBid:   &minBid,
		},
	}

	t.Run("per-key list replaces, min_bid inherits", func(t *testing.T) {
		v := validator{proposerSettings: &proposer.Settings{
			DefaultConfig: defaultOption,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
				pk: {BuilderConfig: &proposer.BuilderConfig{
					Enabled:  proto.Bool(true),
					Builders: []*proposer.BuilderEntry{{URL: "https://c"}},
				}},
			},
		}}
		targets := v.builderTargetsForKey(pk)
		require.Equal(t, 1, len(targets))
		require.Equal(t, "https://c", targets[0].url)
		require.NotNil(t, targets[0].minBid)
		require.Equal(t, uint64(5000000), *targets[0].minBid)
	})
	t.Run("enabled-only per-key inherits the default builder list", func(t *testing.T) {
		v := validator{proposerSettings: &proposer.Settings{
			DefaultConfig: defaultOption,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
				pk: {BuilderConfig: &proposer.BuilderConfig{Enabled: proto.Bool(true)}},
			},
		}}
		require.Equal(t, 2, len(v.builderTargetsForKey(pk)))
	})
	t.Run("explicit zero max payment stays trustless-only", func(t *testing.T) {
		defWithCap := &proposer.Option{BuilderConfig: &proposer.BuilderConfig{
			Enabled:             proto.Bool(true),
			Builders:            []*proposer.BuilderEntry{{URL: "https://a"}},
			MaxExecutionPayment: uint64ValPtrClient(1000000000),
		}}
		v := validator{proposerSettings: &proposer.Settings{
			DefaultConfig: defWithCap,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
				pk: {BuilderConfig: &proposer.BuilderConfig{
					Enabled:             proto.Bool(true),
					MaxExecutionPayment: &zeroPayment,
				}},
			},
		}}
		targets := v.builderTargetsForKey(pk)
		require.Equal(t, 1, len(targets))
		require.Equal(t, uint64(0), targets[0].maxPayment)
	})
	// Pins the full resolution chain: entry field -> effective config -> default_config.
	// Entry X overrides everything; entry Y inherits config-level values, which
	// themselves came from default_config through the coalesce.
	t.Run("entry overrides beat config level, unset entry fields inherit through the chain", func(t *testing.T) {
		defBoost := validatortypes.Uint64(90)
		defMax := validatortypes.Uint64(100)
		entryMin := validatortypes.Uint64(7000000)
		entryBoost := validatortypes.Uint64(120)
		entryMax := validatortypes.Uint64(50)
		v := validator{proposerSettings: &proposer.Settings{
			DefaultConfig: &proposer.Option{BuilderConfig: &proposer.BuilderConfig{
				MinBid:              &minBid, // 5000000
				BuilderBoostFactor:  &defBoost,
				MaxExecutionPayment: &defMax,
			}},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
				pk: {BuilderConfig: &proposer.BuilderConfig{
					Enabled: proto.Bool(true),
					Builders: []*proposer.BuilderEntry{
						{URL: "https://x", MinBid: &entryMin, BuilderBoostFactor: &entryBoost, MaxExecutionPayment: &entryMax},
						{URL: "https://y"},
					},
				}},
			},
		}}
		targets := v.builderTargetsForKey(pk)
		require.Equal(t, 2, len(targets))

		x := targets[0]
		require.Equal(t, "https://x", x.url)
		require.Equal(t, uint64(7000000), *x.minBid)
		require.Equal(t, uint64(120), *x.boostFactor)
		require.Equal(t, uint64(50), x.maxPayment)

		y := targets[1]
		require.Equal(t, "https://y", y.url)
		require.Equal(t, uint64(5000000), *y.minBid)
		require.Equal(t, uint64(90), *y.boostFactor)
		require.Equal(t, uint64(100), y.maxPayment)
	})
	t.Run("per-key disable wins over enabled default", func(t *testing.T) {
		v := validator{proposerSettings: &proposer.Settings{
			DefaultConfig: defaultOption,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
				pk: {BuilderConfig: &proposer.BuilderConfig{Enabled: proto.Bool(false)}},
			},
		}}
		require.Equal(t, 0, len(v.builderTargetsForKey(pk)))
	})
}

func uint64ValPtrClient(v uint64) *validatortypes.Uint64 {
	u := validatortypes.Uint64(v)
	return &u
}

// Warm both signs auths for the inline path and emits the ahead-of-time
// builder-specs preference submissions, pairing each builder URL with the auth
// signed over ITS auth_data and its own max payment.
func TestWarmBuilderRequestAuthsForDuties_EmitsPreferenceSubmissions(t *testing.T) {
	kp := randKeypair(t)
	km := newMockKeymanager(t, kp)
	slot := primitives.Slot(30)
	maxA := validatortypes.Uint64(100)
	maxB := validatortypes.Uint64(200)

	v := validator{proposerSettings: settingsWithBuilders(kp.pub,
		&proposer.BuilderEntry{URL: "https://a", AuthData: []byte("A"), MaxExecutionPayment: &maxA},
		&proposer.BuilderEntry{URL: "https://b", AuthData: []byte("B"), MaxExecutionPayment: &maxB},
	)}
	duties := func(yield func(pubkey, *ethpb.ValidatorDuty) bool) {
		// One past slot (skipped) and one future proposal slot.
		yield(kp.pub, &ethpb.ValidatorDuty{ProposerSlots: []primitives.Slot{slot, slot + 2}})
	}

	reqs := v.warmBuilderRequestAuthsForDuties(t.Context(), km, slot, duties)
	require.Equal(t, 2, len(reqs))

	byURL := map[string]*ethpb.SubmitBuilderPreferencesRequest{}
	for _, r := range reqs {
		byURL[r.Url] = r
		require.DeepEqual(t, kp.pub[:], r.ValidatorPubkey)
		require.Equal(t, slot+2, r.Request.Auth.Message.Slot)
	}
	require.DeepEqual(t, []byte("A"), byURL["https://a"].Request.Auth.Message.Data)
	require.Equal(t, primitives.Gwei(100), byURL["https://a"].Request.Preferences.MaxExecutionPayment)
	require.DeepEqual(t, []byte("B"), byURL["https://b"].Request.Auth.Message.Data)
	require.Equal(t, primitives.Gwei(200), byURL["https://b"].Request.Preferences.MaxExecutionPayment)
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
