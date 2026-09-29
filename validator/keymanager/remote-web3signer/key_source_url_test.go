package remote_web3signer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/remote-web3signer/internal"
)

const (
	testKeyA = "0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"
	testKeyB = "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055"
)

// stubSignerClient is a controllable HttpSignerClient for poller tests: it
// returns whatever (resp, err) is currently set, guarded for concurrent use.
type stubSignerClient struct {
	mu   sync.Mutex
	resp []string
	err  error
}

func (c *stubSignerClient) set(resp []string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resp, c.err = resp, err
}

func (c *stubSignerClient) GetPublicKeys(context.Context, string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resp, c.err
}

func (c *stubSignerClient) Sign(context.Context, string, internal.SignRequestJson) (bls.Signature, error) {
	return nil, nil
}

func decodeKeysMust(t *testing.T, hexes ...string) []pubkey {
	t.Helper()
	keys, err := decodePublicKeys(hexes)
	require.NoError(t, err)
	return keys
}

func newPollTestKeymanager(client internal.HttpSignerClient, initial []pubkey) *Keymanager {
	km := &Keymanager{
		client:              client,
		accountsChangedFeed: new(event.Feed),
	}
	km.keys.replace(sourceURL, initial)
	return km
}

func TestPollRemoteKeysFromURL(t *testing.T) {
	const interval = time.Second

	cases := []struct {
		name       string
		seed       []string
		resp       []string
		respErr    error
		wantChange bool
		wantKeys   int
	}{
		{name: "applies added keys", seed: []string{testKeyA}, resp: []string{testKeyA, testKeyB}, wantChange: true, wantKeys: 2},
		{name: "applies removed keys", seed: []string{testKeyA, testKeyB}, resp: []string{testKeyA}, wantChange: true, wantKeys: 1},
		{name: "keeps keys on client error", seed: []string{testKeyA}, respErr: errors.New("signer unreachable"), wantKeys: 1},
		{name: "keeps keys on empty response", seed: []string{testKeyA}, resp: []string{}, wantKeys: 1},
		{name: "keeps keys on undecodable response", seed: []string{testKeyA}, resp: []string{"not-hex"}, wantKeys: 1},
		{name: "no event when unchanged", seed: []string{testKeyA}, resp: []string{testKeyA}, wantKeys: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				stub := &stubSignerClient{}
				stub.set(tc.resp, tc.respErr)
				km := newPollTestKeymanager(stub, decodeKeysMust(t, tc.seed...))

				ch := make(chan []pubkey, 8)
				sub := km.SubscribeAccountChanges(ch)
				defer sub.Unsubscribe()

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go km.pollRemoteKeysFromURL(ctx, "http://signer/keys", interval)

				// Note: This is instant as we're in a synctest bubble.
				<-time.After(3 * interval)
				synctest.Wait()

				if tc.wantChange {
					select {
					case updated := <-ch:
						require.Equal(t, tc.wantKeys, len(updated))
					default:
						t.Fatal("expected a key-change event")
					}
				} else {
					select {
					case <-ch:
						t.Fatal("did not expect a key-change event")
					default:
					}
				}

				got, err := km.FetchValidatingPublicKeys(ctx)
				require.NoError(t, err)
				require.Equal(t, tc.wantKeys, len(got))
			})
		})
	}

	t.Run("stops on context cancel", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stub := &stubSignerClient{}
			stub.set([]string{testKeyA}, nil)
			km := newPollTestKeymanager(stub, decodeKeysMust(t, testKeyA))

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				km.pollRemoteKeysFromURL(ctx, "http://signer/keys", interval)
				close(done)
			}()

			synctest.Wait() // poller is blocked on its ticker
			cancel()
			synctest.Wait() // poller observes cancellation and returns

			select {
			case <-done:
			default:
				t.Fatal("poller did not stop after context cancellation")
			}
		})
	})

	t.Run("returns immediately when disabled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			km := newPollTestKeymanager(&stubSignerClient{}, nil)
			done := make(chan struct{})
			go func() {
				km.pollRemoteKeysFromURL(context.Background(), "", interval)            // no URL
				km.pollRemoteKeysFromURL(context.Background(), "http://signer/keys", 0) // non-positive interval
				close(done)
			}()

			synctest.Wait()
			select {
			case <-done:
			default:
				t.Fatal("poller did not return when disabled")
			}
		})
	})
}
