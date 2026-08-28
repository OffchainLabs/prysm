package builder

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	builderapi "github.com/OffchainLabs/prysm/v7/api/client/builder"
	buildertesting "github.com/OffchainLabs/prysm/v7/api/client/builder/testing"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// fakeBuilderClient is a per-URL builder client for exercising the multiplex
// fan-out: it returns a configurable bid/error and records calls.
type fakeBuilderClient struct {
	buildertesting.MockClient
	url       string
	bid       *eth.SignedExecutionPayloadBid
	getErr    error
	getCount  atomic.Int32
	prefCount atomic.Int32
}

func (f *fakeBuilderClient) NodeURL() string { return f.url }

func (f *fakeBuilderClient) GetExecutionPayloadBid(context.Context, primitives.Slot, [32]byte, [32]byte, [48]byte, *eth.SignedRequestAuth) (*eth.SignedExecutionPayloadBid, error) {
	f.getCount.Add(1)
	return f.bid, f.getErr
}

func (f *fakeBuilderClient) SubmitBuilderPreferences(context.Context, [48]byte, *eth.BuilderPreferencesRequest) error {
	f.prefCount.Add(1)
	return nil
}

func entryFor(url string) *eth.BuilderEntry {
	return entryWithAuthData(url, url)
}

func entryWithAuthData(url, data string) *eth.BuilderEntry {
	return &eth.BuilderEntry{
		Url:  []byte(url),
		Auth: &eth.SignedRequestAuth{Message: &eth.RequestAuth{Data: []byte(data)}},
	}
}

func bidWithValue(v primitives.Gwei) *eth.SignedExecutionPayloadBid {
	return &eth.SignedExecutionPayloadBid{Message: &eth.ExecutionPayloadBid{Value: v}}
}

func newMultiplexService(t *testing.T, clients map[string]*fakeBuilderClient) *Service {
	s, err := NewService(t.Context())
	require.NoError(t, err)
	s.dial = func(url string) (builderapi.BuilderClient, error) {
		c, ok := clients[url]
		if !ok {
			return nil, errors.New("no client for " + url)
		}
		return c, nil
	}
	return s
}

func TestGetExecutionPayloadBid_FanOutAndDedup(t *testing.T) {
	clients := map[string]*fakeBuilderClient{
		"http://a": {url: "http://a", bid: bidWithValue(100)},
		"http://b": {url: "http://b", bid: bidWithValue(200)},
	}
	s := newMultiplexService(t, clients)

	entries := []*eth.BuilderEntry{entryFor("http://a"), entryFor("http://b"), entryFor("http://a")}
	bids, err := s.GetExecutionPayloadBid(t.Context(), 1, [32]byte{}, [32]byte{}, [48]byte{}, entries)
	require.NoError(t, err)
	require.Equal(t, 2, len(bids))
	require.Equal(t, int32(1), clients["http://a"].getCount.Load())

	got := map[string]primitives.Gwei{}
	for _, pb := range bids {
		got[string(pb.Entry.GetUrl())] = pb.Bid.Message.Value
	}
	require.Equal(t, primitives.Gwei(100), got["http://a"])
	require.Equal(t, primitives.Gwei(200), got["http://b"])
}

func TestGetExecutionPayloadBid_SharedURLDistinctAuthData(t *testing.T) {
	proxy := &fakeBuilderClient{url: "http://proxy", bid: bidWithValue(10)}
	s := newMultiplexService(t, map[string]*fakeBuilderClient{"http://proxy": proxy})

	entries := []*eth.BuilderEntry{
		entryWithAuthData("http://proxy", "builder-1"),
		entryWithAuthData("http://proxy", "builder-2"),
		entryWithAuthData("http://proxy", "builder-1"),
	}
	bids, err := s.GetExecutionPayloadBid(t.Context(), 1, [32]byte{}, [32]byte{}, [48]byte{}, entries)
	require.NoError(t, err)
	require.Equal(t, 2, len(bids))
	require.Equal(t, int32(2), proxy.getCount.Load())
}

func TestGetExecutionPayloadBid_SkipsErrorsAndNil(t *testing.T) {
	clients := map[string]*fakeBuilderClient{
		"http://ok":   {url: "http://ok", bid: bidWithValue(50)},
		"http://err":  {url: "http://err", getErr: errors.New("boom")},
		"http://none": {url: "http://none", bid: nil},
	}
	s := newMultiplexService(t, clients)

	// http://nodial has no client; dialing it fails and is skipped.
	entries := []*eth.BuilderEntry{entryFor("http://ok"), entryFor("http://err"), entryFor("http://none"), entryFor("http://nodial")}
	bids, err := s.GetExecutionPayloadBid(t.Context(), 1, [32]byte{}, [32]byte{}, [48]byte{}, entries)
	require.NoError(t, err)
	require.Equal(t, 1, len(bids))
	require.Equal(t, "http://ok", string(bids[0].Entry.GetUrl()))
}

func TestGetExecutionPayloadBid_NoEntries(t *testing.T) {
	s := newMultiplexService(t, nil)
	bids, err := s.GetExecutionPayloadBid(t.Context(), 1, [32]byte{}, [32]byte{}, [48]byte{}, nil)
	require.NoError(t, err)
	require.Equal(t, 0, len(bids))
}

func TestClientFor_SeedsFlagClientAndCachesDials(t *testing.T) {
	seed := &fakeBuilderClient{url: "http://seed"}
	s, err := NewService(t.Context(), WithBuilderClient(seed))
	require.NoError(t, err)

	dialed := 0
	s.dial = func(url string) (builderapi.BuilderClient, error) {
		dialed++
		return &fakeBuilderClient{url: url}, nil
	}

	// The flag client seeds the map, so its URL is served without dialing.
	c, err := s.clientFor("http://seed")
	require.NoError(t, err)
	require.Equal(t, "http://seed", c.NodeURL())
	require.Equal(t, 0, dialed)

	// A new URL dials once and is then cached.
	_, err = s.clientFor("http://new")
	require.NoError(t, err)
	_, err = s.clientFor("http://new")
	require.NoError(t, err)
	require.Equal(t, 1, dialed)

	// Invalid urls are rejected before dialing.
	_, err = s.clientFor("ftp://elsewhere")
	require.ErrorContains(t, "scheme must be http or https", err)
	require.Equal(t, 1, dialed)
}

func TestValidBuilderURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "http", url: "http://builder.example:8080"},
		{name: "https", url: "https://builder.example"},
		{name: "host port", url: "10.0.0.1:8080"},
		{name: "bad scheme", url: "ftp://builder.example", wantErr: "scheme must be http or https"},
		{name: "opaque scheme", url: "javascript:alert(1)", wantErr: "malformed builder url"},
		{name: "no host", url: "http://", wantErr: "malformed builder url"},
		{name: "empty", url: "", wantErr: "malformed builder url"},
		{name: "bare host", url: "builder.example", wantErr: "malformed builder url"},
		{name: "non-ascii path", url: "http://builder.example/π", wantErr: "malformed builder url"},
		{name: "embedded space", url: "http://builder.example/a b", wantErr: "malformed builder url"},
		{name: "control char", url: "http://builder.example/a\r\nb", wantErr: "malformed builder url"},
		{name: "percent-encoded", url: "http://builder.example/%0d%0a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validBuilderURL(tc.url)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, tc.wantErr, err)
			}
		})
	}
}

func TestSubmitClientFor(t *testing.T) {
	newService := func(t *testing.T, cached map[string]*fakeBuilderClient) (*Service, *int) {
		s := newMultiplexService(t, nil)
		dialed := 0
		s.dial = func(url string) (builderapi.BuilderClient, error) {
			dialed++
			return &fakeBuilderClient{url: url}, nil
		}
		for u, c := range cached {
			s.clients[u] = c
		}
		return s, &dialed
	}

	t.Run("cached client is served without dialing", func(t *testing.T) {
		s, dialed := newService(t, map[string]*fakeBuilderClient{"http://cached": {url: "http://cached"}})
		c, err := s.submitClientFor("http://cached")
		require.NoError(t, err)
		require.Equal(t, "http://cached", c.NodeURL())
		require.Equal(t, 0, *dialed)
	})

	t.Run("uncached url dials but is not cached", func(t *testing.T) {
		s, dialed := newService(t, nil)
		_, err := s.submitClientFor("http://transient")
		require.NoError(t, err)
		_, err = s.submitClientFor("http://transient")
		require.NoError(t, err)
		require.Equal(t, 2, *dialed)
		require.Equal(t, 0, len(s.clients))
	})

	t.Run("invalid url is rejected before dialing", func(t *testing.T) {
		s, dialed := newService(t, nil)
		_, err := s.submitClientFor("gopher://elsewhere")
		require.ErrorContains(t, "scheme must be http or https", err)
		require.Equal(t, 0, *dialed)
	})
}

func TestSubmitBuilderPreferences(t *testing.T) {
	prefEntry := func(url string) *eth.BuilderPreferencesEntry {
		return &eth.BuilderPreferencesEntry{
			Url:                 []byte(url),
			MaxExecutionPayment: 5,
			Auth:                &eth.SignedRequestAuth{Message: &eth.RequestAuth{Data: []byte("opaque-auth-data")}},
		}
	}

	t.Run("failures are keyed by entry position", func(t *testing.T) {
		fc := &fakeBuilderClient{url: "http://ok"}
		s := newMultiplexService(t, map[string]*fakeBuilderClient{"http://ok": fc})
		failures := s.SubmitBuilderPreferences(t.Context(), []*eth.BuilderPreferencesEntry{
			prefEntry(""), prefEntry("http://ok"), prefEntry("http://nodial"),
		})
		require.Equal(t, 2, len(failures))
		require.StringContains(t, "builder url is required", failures[0])
		require.StringContains(t, "could not submit builder preferences", failures[2])
		require.Equal(t, int32(1), fc.prefCount.Load())
	})

	t.Run("nil entries are skipped", func(t *testing.T) {
		fc := &fakeBuilderClient{url: "http://ok"}
		s := newMultiplexService(t, map[string]*fakeBuilderClient{"http://ok": fc})
		failures := s.SubmitBuilderPreferences(t.Context(), []*eth.BuilderPreferencesEntry{nil, prefEntry("http://ok")})
		require.Equal(t, 0, len(failures))
		require.Equal(t, int32(1), fc.prefCount.Load())
	})

	t.Run("no failures on success", func(t *testing.T) {
		fc := &fakeBuilderClient{url: "http://ok"}
		s := newMultiplexService(t, map[string]*fakeBuilderClient{"http://ok": fc})
		failures := s.SubmitBuilderPreferences(t.Context(), []*eth.BuilderPreferencesEntry{prefEntry("http://ok")})
		require.Equal(t, 0, len(failures))
	})
}
