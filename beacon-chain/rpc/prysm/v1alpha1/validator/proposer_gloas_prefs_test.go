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

func TestBuilderAuthsAndPrefs(t *testing.T) {
	minBid := uint64(7)
	boost := uint64(120)

	t.Run("empty input yields no auths and neutral prefs", func(t *testing.T) {
		auths, bp := builderAuthsAndPrefs(nil)
		require.Equal(t, 0, len(auths))
		require.Equal(t, uint64(0), bp.maxPayment)
		require.Equal(t, uint64(0), bp.minBid)
		require.Equal(t, uint64(neutralBuilderBoostFactor), bp.boostFactor)
	})
	t.Run("malformed entries are skipped and never set the baseline", func(t *testing.T) {
		badMin := uint64(999)
		entries := []*ethpb.BuilderPreferenceV1{
			nil,
			{Url: "https://no-request", MinBid: func() *primitives.Gwei { g := primitives.Gwei(badMin); return &g }()},
			{Url: "https://no-auth", Request: &ethpb.BuilderPreferencesRequestV1{}},
			func() *ethpb.BuilderPreferenceV1 { p := prefEntry("", 5, &badMin, nil); return p }(),
			prefEntry("https://good", 42, &minBid, &boost),
		}
		auths, bp := builderAuthsAndPrefs(entries)
		require.Equal(t, 1, len(auths))
		require.NotNil(t, auths["https://good"])
		require.Equal(t, uint64(42), bp.maxPayment)
		require.Equal(t, minBid, bp.minBid)
		require.Equal(t, boost, bp.boostFactor)
	})
	t.Run("first valid entry sets the baseline, later entries do not override", func(t *testing.T) {
		otherMin := uint64(1)
		otherBoost := uint64(999)
		auths, bp := builderAuthsAndPrefs([]*ethpb.BuilderPreferenceV1{
			prefEntry("https://a", 100, &minBid, &boost),
			prefEntry("https://b", 200, &otherMin, &otherBoost),
		})
		require.Equal(t, 2, len(auths))
		require.Equal(t, uint64(100), bp.maxPayment)
		require.Equal(t, minBid, bp.minBid)
		require.Equal(t, boost, bp.boostFactor)
	})
	t.Run("duplicate url keeps the first auth", func(t *testing.T) {
		first := prefEntry("https://dup", 1, nil, nil)
		second := prefEntry("https://dup", 2, nil, nil)
		auths, _ := builderAuthsAndPrefs([]*ethpb.BuilderPreferenceV1{first, second})
		require.Equal(t, 1, len(auths))
		require.Equal(t, first.Request.Auth, auths["https://dup"])
	})
	t.Run("unset min_bid and boost fall to client defaults", func(t *testing.T) {
		_, bp := builderAuthsAndPrefs([]*ethpb.BuilderPreferenceV1{prefEntry("https://a", 5, nil, nil)})
		require.Equal(t, uint64(5), bp.maxPayment)
		require.Equal(t, uint64(0), bp.minBid)
		require.Equal(t, uint64(neutralBuilderBoostFactor), bp.boostFactor)
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
