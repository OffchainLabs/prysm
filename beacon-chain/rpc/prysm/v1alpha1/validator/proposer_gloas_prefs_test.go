package validator

import (
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func prefEntry(url string, maxPayment uint64, minBid *uint64, boost *uint64) *ethpb.BuilderPreferenceV1 {
	p := &ethpb.BuilderPreferenceV1{
		Url: url,
		Request: &ethpb.BuilderPreferencesRequestV1{
			Preferences: &ethpb.BuilderPreferencesV1{MaxExecutionPayment: primitives.Gwei(maxPayment)},
			Auth:        &ethpb.SignedRequestAuthV1{Message: &ethpb.RequestAuthV1{Data: []byte(url)}},
		},
	}
	if minBid != nil {
		g := primitives.Gwei(*minBid)
		p.MinBid = &g
	}
	p.BuilderBoostFactor = boost
	return p
}

func TestBuilderPrefsByURL(t *testing.T) {
	minBid := uint64(7)
	boost := uint64(120)

	t.Run("empty input yields empty map", func(t *testing.T) {
		require.Equal(t, 0, len(builderPrefsByURL(nil)))
	})
	t.Run("each url keeps its own preferences", func(t *testing.T) {
		otherMin := uint64(1)
		otherBoost := uint64(999)
		out := builderPrefsByURL([]*ethpb.BuilderPreferenceV1{
			prefEntry("https://a", 100, &minBid, &boost),
			prefEntry("https://b", 200, &otherMin, &otherBoost),
		})
		require.Equal(t, 2, len(out))
		require.Equal(t, uint64(100), out["https://a"].prefs.maxPayment)
		require.Equal(t, minBid, out["https://a"].prefs.minBid)
		require.Equal(t, boost, out["https://a"].prefs.boostFactor)
		require.Equal(t, uint64(200), out["https://b"].prefs.maxPayment)
		require.Equal(t, otherMin, out["https://b"].prefs.minBid)
		require.Equal(t, otherBoost, out["https://b"].prefs.boostFactor)
	})
	t.Run("malformed entries are skipped", func(t *testing.T) {
		badMin := uint64(999)
		out := builderPrefsByURL([]*ethpb.BuilderPreferenceV1{
			nil,
			{Url: "https://no-request", MinBid: func() *primitives.Gwei { g := primitives.Gwei(badMin); return &g }()},
			{Url: "https://no-auth", Request: &ethpb.BuilderPreferencesRequestV1{}},
			func() *ethpb.BuilderPreferenceV1 { p := prefEntry("", 5, &badMin, nil); return p }(),
			prefEntry("https://good", 42, &minBid, &boost),
		})
		require.Equal(t, 1, len(out))
		require.NotNil(t, out["https://good"].auth)
	})
	t.Run("duplicate url keeps the first entry", func(t *testing.T) {
		out := builderPrefsByURL([]*ethpb.BuilderPreferenceV1{
			prefEntry("https://dup", 1, nil, nil),
			prefEntry("https://dup", 2, nil, nil),
		})
		require.Equal(t, 1, len(out))
		require.Equal(t, uint64(1), out["https://dup"].prefs.maxPayment)
	})
	t.Run("unset min_bid and boost fall to client defaults", func(t *testing.T) {
		out := builderPrefsByURL([]*ethpb.BuilderPreferenceV1{prefEntry("https://a", 5, nil, nil)})
		require.Equal(t, uint64(5), out["https://a"].prefs.maxPayment)
		require.Equal(t, uint64(0), out["https://a"].prefs.minBid)
		require.Equal(t, uint64(neutralBuilderBoostFactor), out["https://a"].prefs.boostFactor)
	})
}

func TestValidBuilderURL(t *testing.T) {
	valid := []string{"https://builder.example", "http://10.0.0.5:18550", "https://user@builder.example/path"}
	for _, u := range valid {
		require.Equal(t, true, validBuilderURL(u), u)
	}
	invalid := []string{"", "not a url", "file:///etc/passwd", "gopher://x", "https://", "//missing-scheme"}
	for _, u := range invalid {
		require.Equal(t, false, validBuilderURL(u), u)
	}
}

// The masking fallback echoes raw input on parse failure, so unparseable values
// must never reach it.
func TestSafeURLForLog(t *testing.T) {
	// Space in host is one of the few inputs url.Parse rejects outright.
	unparseable := "http://user:secret@ho st"
	require.Equal(t, "<unparseable>", safeURLForLog(unparseable))
	require.Equal(t, false, strings.Contains(safeURLForLog(unparseable), "secret"))
	require.Equal(t, false, strings.Contains(safeURLForLog("https://user:secret@builder.example/path"), "secret"))
}
