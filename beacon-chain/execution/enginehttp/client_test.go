package enginehttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// 32-byte HS256 secret shared by the test client and server.
var testSecret = []byte("0123456789abcdef0123456789abcdef")

// stubSSZ is a minimal type implementing the fastssz Marshaler/Unmarshaler
// interfaces.
type stubSSZ struct{ data []byte }

func (s *stubSSZ) MarshalSSZ() ([]byte, error)             { return s.data, nil }
func (s *stubSSZ) MarshalSSZTo(dst []byte) ([]byte, error) { return append(dst, s.data...), nil }
func (s *stubSSZ) SizeSSZ() int                            { return len(s.data) }
func (s *stubSSZ) UnmarshalSSZ(buf []byte) error {
	s.data = slices.Clone(buf)
	return nil
}

// testClient stands up an h2c server wrapping handler and returns a Client
// pointed at it.
func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	srv := httptest.NewServer(h2c.NewHandler(handler, &http2.Server{}))
	t.Cleanup(srv.Close)
	c, err := New(Config{
		BaseURL:       srv.URL,
		JWTSecret:     testSecret,
		ClientVersion: "Prysm/test",
	})
	require.NoError(t, err)
	return c
}

// verifyJWT parses and validates the bearer token using the shared secret,
// exercising the per-request signing path end-to-end.
func verifyJWT(r *http.Request) bool {
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return false
	}
	tok, err := jwt.Parse(parts[1], func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return testSecret, nil
	})
	if err != nil || !tok.Valid {
		return false
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return false
	}
	_, ok = claims["iat"]
	return ok
}

func TestSSZRequest_Post(t *testing.T) {
	reqBytes := []byte("ssz-request-body")
	respBytes := []byte("ssz-response-body")

	var gotMethod, gotPath, gotCT, gotAccept, gotCV string
	var gotProtoMajor int
	var gotBody []byte
	var jwtOK bool
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		gotCV = r.Header.Get(clientVersionHeader)
		gotProtoMajor = r.ProtoMajor
		gotBody, _ = io.ReadAll(r.Body)
		jwtOK = verifyJWT(r)
		w.Header().Set("Content-Type", contentTypeSSZ)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBytes)
	})

	out := &stubSSZ{}
	err := c.SSZRequest(context.Background(), http.MethodPost, "/amsterdam/payloads", nil, &stubSSZ{data: reqBytes}, out)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/engine/v2/amsterdam/payloads", gotPath)
	assert.Equal(t, contentTypeSSZ, gotCT)
	assert.Equal(t, contentTypeSSZ, gotAccept)
	assert.Equal(t, "Prysm/test", gotCV)
	assert.Equal(t, 2, gotProtoMajor) // h2c negotiated HTTP/2
	assert.Equal(t, true, jwtOK)
	assert.DeepEqual(t, reqBytes, gotBody)
	assert.DeepEqual(t, respBytes, out.data)
}

func TestSSZRequest_GetNoBody(t *testing.T) {
	respBytes := []byte("built-payload-bytes")

	var gotMethod, gotCT string
	var hadBody bool
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		hadBody = len(b) > 0
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBytes)
	})

	out := &stubSSZ{}
	err := c.SSZRequest(context.Background(), http.MethodGet, "/amsterdam/payloads/0x0123456789abcdef", nil, nil, out)
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "", gotCT) // no Content-Type when there is no request body
	assert.Equal(t, false, hadBody)
	assert.DeepEqual(t, respBytes, out.data)
}

func TestSSZRequest_Query(t *testing.T) {
	var gotQuery url.Values
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
	})

	q := url.Values{}
	q.Set("from", "1")
	q.Set("count", "2")
	err := c.SSZRequest(context.Background(), http.MethodGet, "/amsterdam/bodies", q, nil, &stubSSZ{})
	require.NoError(t, err)

	assert.Equal(t, "1", gotQuery.Get("from"))
	assert.Equal(t, "2", gotQuery.Get("count"))
}

func TestSSZRequest_NoContent(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.SSZRequest(context.Background(), http.MethodPost, "/blobs/v1", nil, &stubSSZ{data: []byte("x")}, &stubSSZ{})
	require.NotNil(t, err)
	require.Equal(t, true, errors.Is(err, ErrNoContent))
}

func TestSSZRequest_ProblemJSON(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"type":"/engine-api/errors/invalid-forkchoice","detail":"finalized not ancestor of head"}`))
	})

	err := c.SSZRequest(context.Background(), http.MethodPost, "/amsterdam/forkchoice", nil, &stubSSZ{data: []byte("x")}, &stubSSZ{})
	require.NotNil(t, err)

	var apiErr *Error
	require.Equal(t, true, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusConflict, apiErr.Status)
	assert.Equal(t, ProblemInvalidForkchoice, apiErr.Problem.Type)
	assert.Equal(t, "finalized not ancestor of head", apiErr.Problem.Detail)
}

func TestSSZRequest_NonJSONErrorBody(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	})

	err := c.SSZRequest(context.Background(), http.MethodPost, "/amsterdam/payloads", nil, &stubSSZ{data: []byte("x")}, &stubSSZ{})
	require.NotNil(t, err)

	var apiErr *Error
	require.Equal(t, true, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusInternalServerError, apiErr.Status)
	assert.Equal(t, "boom", apiErr.RawBody)
	assert.Equal(t, true, strings.Contains(apiErr.Error(), "500"))
}

func TestJSONGet(t *testing.T) {
	var gotAccept, gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", contentTypeJSON)
		_, _ = w.Write([]byte(`{"supported_forks":["amsterdam"],"limits":{"payload.max_bytes":268435456}}`))
	})

	var caps struct {
		SupportedForks []string         `json:"supported_forks"`
		Limits         map[string]int64 `json:"limits"`
	}
	err := c.JSONGet(context.Background(), "/capabilities", &caps)
	require.NoError(t, err)

	assert.Equal(t, contentTypeJSON, gotAccept)
	assert.Equal(t, "/engine/v2/capabilities", gotPath)
	require.Equal(t, 1, len(caps.SupportedForks))
	assert.Equal(t, "amsterdam", caps.SupportedForks[0])
	assert.Equal(t, int64(268435456), caps.Limits["payload.max_bytes"])
}

func TestNew_Validation(t *testing.T) {
	_, err := New(Config{BaseURL: "", JWTSecret: testSecret})
	require.NotNil(t, err)

	_, err = New(Config{BaseURL: "https://host:8551", JWTSecret: testSecret})
	require.NotNil(t, err) // h2c only; https rejected

	_, err = New(Config{BaseURL: "http://host:8551"})
	require.NotNil(t, err) // empty secret

	_, err = New(Config{BaseURL: "http://host:8551", JWTSecret: testSecret})
	require.NoError(t, err)
}
