package builder

import (
	"context"
	"errors"
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
	getCount  int
	prefCount int
}

func (f *fakeBuilderClient) NodeURL() string { return f.url }

func (f *fakeBuilderClient) GetExecutionPayloadBid(context.Context, primitives.Slot, [32]byte, [32]byte, [48]byte, *eth.SignedRequestAuth) (*eth.SignedExecutionPayloadBid, error) {
	f.getCount++
	return f.bid, f.getErr
}

func (f *fakeBuilderClient) SubmitBuilderPreferences(context.Context, [48]byte, *eth.BuilderPreferencesRequest) error {
	f.prefCount++
	return nil
}

func entryFor(url string) *eth.BuilderEntry {
	return entryWithAuthData(url, url)
}

func entryWithAuthData(url, data string) *eth.BuilderEntry {
	return &eth.BuilderEntry{
		Url:  url,
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
	require.Equal(t, 1, clients["http://a"].getCount)

	got := map[string]primitives.Gwei{}
	for _, pb := range bids {
		got[pb.BuilderURL] = pb.Bid.Message.Value
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
	require.Equal(t, 2, proxy.getCount)
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
	require.Equal(t, "http://ok", bids[0].BuilderURL)
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
}

func TestSubmitBuilderPreferences_DialsPerURL(t *testing.T) {
	fc := &fakeBuilderClient{url: "http://b"}
	s := newMultiplexService(t, map[string]*fakeBuilderClient{"http://b": fc})

	req := &eth.BuilderPreferencesRequest{
		Preferences: &eth.BuilderPreferences{},
		Auth:        &eth.SignedRequestAuth{Message: &eth.RequestAuth{Data: []byte("opaque-auth-data")}},
	}
	require.NoError(t, s.SubmitBuilderPreferences(t.Context(), [48]byte{}, "http://b", req))
	require.Equal(t, 1, fc.prefCount)

	err := s.SubmitBuilderPreferences(t.Context(), [48]byte{}, "", req)
	require.ErrorContains(t, "missing builder url", err)
}
