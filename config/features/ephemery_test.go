package features

import (
	"bytes"
	"embed"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

//go:embed testdata/ephemery_config.yaml testdata/ephemery_boot_enr.txt
var ephemeryTestdata embed.FS

func ephemeryFixture(t *testing.T, name string) []byte {
	b, err := ephemeryTestdata.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

// ephemeryServer serves the given bodies by path, falling back to the recorded fixtures.
func ephemeryServer(t *testing.T, bodies map[string][]byte) string {
	files := map[string][]byte{
		"/config.yaml":  ephemeryFixture(t, "ephemery_config.yaml"),
		"/boot_enr.txt": ephemeryFixture(t, "ephemery_boot_enr.txt"),
	}
	maps.Copy(files, bodies)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, err := w.Write(body)
		require.NoError(t, err)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestUseEphemeryNetworkConfig(t *testing.T) {
	t.Run("activates the fetched config and bootnodes", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		require.NoError(t, useEphemeryNetworkConfig(t.Context(), ephemeryServer(t, nil)))

		cfg := params.BeaconConfig()
		require.Equal(t, params.EphemeryName, cfg.ConfigName)
		require.Equal(t, uint64(39438162), cfg.DepositChainID)
		require.Equal(t, uint64(39438162), cfg.DepositNetworkID)
		require.Equal(t, uint64(1785438000), cfg.MinGenesisTime)
		require.Equal(t, uint64(64), cfg.MinGenesisActiveValidatorCount)
		require.DeepEqual(t, []byte{0x10, 0x00, 0x10, 0x1b}, cfg.GenesisForkVersion)
		require.DeepEqual(t, []byte{0x70, 0x00, 0x10, 0x1b}, cfg.FuluForkVersion)
		// The fetched config stops at Fulu, so Gloas continues Ephemery's fork version numbering.
		require.DeepEqual(t, []byte{0x80, 0x00, 0x10, 0x1b}, cfg.GloasForkVersion)

		// The config is reachable by name and by the fork versions it declares.
		byName, err := params.ByName(params.EphemeryName)
		require.NoError(t, err)
		require.Equal(t, params.EphemeryName, byName.ConfigName)
		byVersion, err := params.ByVersion([4]byte{0x10, 0x00, 0x10, 0x1b})
		require.NoError(t, err)
		require.Equal(t, params.EphemeryName, byVersion.ConfigName)

		nodes := params.BeaconNetworkConfig().BootstrapNodes
		require.Equal(t, 3, len(nodes))
		require.Equal(t, "enr:-Iq4QIc297-de1P6hznMX2cIdVsQkve9BD9NUsJ7vVQa7eh5UpekA9rLid5A-yLiS3gZwOGugYZPi58x76zNs2cEQFCGAYhBJlTYgmlkgnY0gmlwhEFtmi6Jc2VjcDI1NmsxoQJDyix-IHa_mVwLBEN9NeG8I-RUjNQK_MGxk9OqRQUAtIN1ZHCCIyg", nodes[2])
		require.Equal(t, uint64(0), params.BeaconNetworkConfig().ContractDeploymentBlock)
	})

	t.Run("ignores a commented out fork version", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		y := append(ephemeryFixture(t, "ephemery_config.yaml"), []byte("\n# GLOAS_FORK_VERSION: 0x9000101b\n")...)
		url := ephemeryServer(t, map[string][]byte{"/config.yaml": y})

		require.NoError(t, useEphemeryNetworkConfig(t.Context(), url))
		require.DeepEqual(t, []byte{0x80, 0x00, 0x10, 0x1b}, params.BeaconConfig().GloasForkVersion)
	})

	t.Run("errors when the config cannot be fetched", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()

		err := useEphemeryNetworkConfig(t.Context(), srv.URL)
		require.ErrorContains(t, "unexpected status", err)
		require.ErrorContains(t, "config.yaml", err)
		// The active config is untouched when the fetch fails.
		require.Equal(t, params.MainnetName, params.BeaconConfig().ConfigName)
	})

	t.Run("errors on a malformed value of a known key", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		y := bytes.Replace(ephemeryFixture(t, "ephemery_config.yaml"),
			[]byte("DEPOSIT_CHAIN_ID: 39438162"), []byte("DEPOSIT_CHAIN_ID: not-a-number"), 1)
		url := ephemeryServer(t, map[string][]byte{"/config.yaml": y})

		// A malformed known value must not silently fall back to the mainnet default.
		err := useEphemeryNetworkConfig(t.Context(), url)
		require.ErrorContains(t, "cannot unmarshal !!str `not-a-n...` into uint64", err)
		require.Equal(t, params.MainnetName, params.BeaconConfig().ConfigName)
	})

	t.Run("errors when the bootnode list is an error page", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		html := []byte("<html>\n<body>\n<h1>404 Not Found</h1>\n</body>\n</html>\n")
		url := ephemeryServer(t, map[string][]byte{"/boot_enr.txt": html})

		err := useEphemeryNetworkConfig(t.Context(), url)
		require.ErrorContains(t, "no valid Ephemery bootnodes found", err)
		// The chain config is only activated once the bootnodes are known to be usable.
		require.Equal(t, params.MainnetName, params.BeaconConfig().ConfigName)
	})

	t.Run("errors on an oversized file", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		url := ephemeryServer(t, map[string][]byte{"/config.yaml": bytes.Repeat([]byte("a"), ephemeryMaxFileSize+1)})

		err := useEphemeryNetworkConfig(t.Context(), url)
		require.ErrorContains(t, "larger than the", err)
	})
}

func TestFetchEphemeryBootnodes(t *testing.T) {
	enr := strings.TrimSpace(strings.Split(string(ephemeryFixture(t, "ephemery_boot_enr.txt")), "\n")[0])

	t.Run("keeps valid ENRs and drops everything else", func(t *testing.T) {
		body := []byte("# a comment\n\n" + enr + "\r\nenr:-not-a-real-enr\nnot-an-enr-at-all\n")
		url := ephemeryServer(t, map[string][]byte{"/boot_enr.txt": body})

		nodes, err := fetchEphemeryBootnodes(t.Context(), url+"/boot_enr.txt")
		require.NoError(t, err)
		require.DeepEqual(t, []string{enr}, nodes)
	})

	t.Run("errors when no line is a valid ENR", func(t *testing.T) {
		url := ephemeryServer(t, map[string][]byte{"/boot_enr.txt": []byte("# only a comment\n\n")})

		_, err := fetchEphemeryBootnodes(t.Context(), url+"/boot_enr.txt")
		require.ErrorContains(t, "no valid Ephemery bootnodes found", err)
	})
}
