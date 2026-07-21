package validator

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// The produce-block body is a bare BuilderEntry array; malformed entries are
// ignored per beacon-APIs #630 rather than failing the request.
func TestParseBuilderPreferencesBody(t *testing.T) {
	validEntry := `{"url":"https://b","auth":{"message":{"data":"0x0102","slot":"7"},"signature":"0x` + repeatHex(96) + `"},"max_execution_payment":"1000","min_bid":"5","builder_boost_factor":"120"}`

	t.Run("valid entry parses with all fields", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`[`+validEntry+`]`))
		prefs, err := parseBuilderPreferencesBody(r)
		require.NoError(t, err)
		require.Equal(t, 1, len(prefs))
		p := prefs[0]
		require.Equal(t, "https://b", p.Url)
		require.Equal(t, uint64(1000), uint64(p.Request.Preferences.MaxExecutionPayment))
		require.Equal(t, uint64(5), uint64(*p.MinBid))
		require.Equal(t, uint64(120), *p.BuilderBoostFactor)
		require.DeepEqual(t, []byte{1, 2}, p.Request.Auth.Message.Data)
	})
	t.Run("malformed entries are skipped, valid ones kept", func(t *testing.T) {
		body := `[{"url":""},{"url":"https://no-auth"},{"url":"https://bad","auth":{"message":{"data":"nothex","slot":"1"},"signature":"0x00"}},` + validEntry + `]`
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
		prefs, err := parseBuilderPreferencesBody(r)
		require.NoError(t, err)
		require.Equal(t, 1, len(prefs))
		require.Equal(t, "https://b", prefs[0].Url)
	})
	t.Run("empty body yields no preferences", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", bytes.NewBuffer(nil))
		prefs, err := parseBuilderPreferencesBody(r)
		require.NoError(t, err)
		require.Equal(t, 0, len(prefs))
	})
	t.Run("non-array body errors", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"builder_preferences":[]}`))
		_, err := parseBuilderPreferencesBody(r)
		require.NotNil(t, err)
	})
}

func repeatHex(n int) string {
	out := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		out = append(out, '0', '9')
	}
	return string(out)
}

// Two entries sharing a url invalidate the whole request, unlike malformed entries.
func TestParseBuilderPreferencesBody_DuplicateURL(t *testing.T) {
	entry := `{"url":"https://b","auth":{"message":{"data":"0x01","slot":"7"},"signature":"0x` + repeatHex(96) + `"}}`
	r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`[`+entry+`,`+entry+`]`))
	_, err := parseBuilderPreferencesBody(r)
	require.ErrorContains(t, "share the same url", err)
}
