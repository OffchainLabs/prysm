package genesis_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/genesis"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestURLProvider(t *testing.T) {
	t.Run("downloads and unmarshals the genesis state", func(t *testing.T) {
		want := createTestGenesisState(t, 4, 0)
		sb, err := want.MarshalSSZ()
		require.NoError(t, err)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write(sb)
			require.NoError(t, err)
		}))
		defer srv.Close()

		got, err := genesis.NewURLProvider(srv.URL+"/genesis.ssz", genesis.DownloadTimeout).Genesis(t.Context())
		require.NoError(t, err)
		require.Equal(t, want.GenesisTime(), got.GenesisTime())
		require.DeepEqual(t, want.GenesisValidatorsRoot(), got.GenesisValidatorsRoot())
	})

	t.Run("errors on an unexpected status", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()

		_, err := genesis.NewURLProvider(srv.URL+"/genesis.ssz", genesis.DownloadTimeout).Genesis(t.Context())
		require.ErrorContains(t, "unexpected status", err)
	})

	t.Run("gives up on a stalled server", func(t *testing.T) {
		block := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			<-block
		}))
		defer func() {
			close(block)
			srv.Close()
		}()

		_, err := genesis.NewURLProvider(srv.URL+"/genesis.ssz", 50*time.Millisecond).Genesis(t.Context())
		require.ErrorContains(t, "context deadline exceeded", err)
	})
}
