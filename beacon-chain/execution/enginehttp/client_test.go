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
	"sync/atomic"
	"testing"
	"time"

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

// testClientMax is like testClient but caps response bodies at max bytes.
func testClientMax(t *testing.T, max int64, handler http.HandlerFunc) *Client {
	srv := httptest.NewServer(h2c.NewHandler(handler, &http2.Server{}))
	t.Cleanup(srv.Close)
	c, err := New(Config{
		BaseURL:          srv.URL,
		JWTSecret:        testSecret,
		ClientVersion:    "Prysm/test",
		MaxResponseBytes: max,
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

func TestRoundtrip_ContentLengthExceedsMax(t *testing.T) {
	// EL declares a Content-Length above the cap: reject before reading the body.
	c := testClientMax(t, 1024, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentTypeSSZ)
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 4096))
	})

	err := c.SSZRequest(context.Background(), http.MethodGet, "/amsterdam/payloads/0x01", nil, nil, &stubSSZ{})
	require.NotNil(t, err)
	assert.Equal(t, true, strings.Contains(err.Error(), "Content-Length"))
}

func TestRoundtrip_BodyExceedsMax(t *testing.T) {
	// EL streams (flushed) chunks so the client sees no Content-Length; the
	// bounded read must still reject a body past the cap.
	c := testClientMax(t, 16, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentTypeSSZ)
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		for range 10 {
			_, _ = w.Write(make([]byte, 8))
			if fl != nil {
				fl.Flush()
			}
		}
	})

	err := c.SSZRequest(context.Background(), http.MethodGet, "/amsterdam/payloads/0x01", nil, nil, &stubSSZ{})
	require.NotNil(t, err)
	assert.Equal(t, true, strings.Contains(err.Error(), "exceeds max"))
}

func TestRoundtrip_BodyAtMax(t *testing.T) {
	// A body exactly at the cap is accepted.
	respBytes := make([]byte, 16)
	c := testClientMax(t, 16, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentTypeSSZ)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBytes)
	})

	out := &stubSSZ{}
	err := c.SSZRequest(context.Background(), http.MethodGet, "/amsterdam/payloads/0x01", nil, nil, out)
	require.NoError(t, err)
	assert.Equal(t, 16, len(out.data))
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Unix(1_000_000, 0)

	d, ok := parseRetryAfter("5", now)
	assert.Equal(t, true, ok)
	assert.Equal(t, 5*time.Second, d)

	d, ok = parseRetryAfter("0", now) // retry immediately
	assert.Equal(t, true, ok)
	assert.Equal(t, time.Duration(0), d)

	d, ok = parseRetryAfter(" 12 ", now) // surrounding space tolerated
	assert.Equal(t, true, ok)
	assert.Equal(t, 12*time.Second, d)

	// HTTP-date form 30s in the future.
	d, ok = parseRetryAfter(now.Add(30*time.Second).UTC().Format(http.TimeFormat), now)
	assert.Equal(t, true, ok)
	assert.Equal(t, 30*time.Second, d)

	// A past HTTP-date is understood but means retry now.
	d, ok = parseRetryAfter(now.Add(-30*time.Second).UTC().Format(http.TimeFormat), now)
	assert.Equal(t, true, ok)
	assert.Equal(t, time.Duration(0), d)

	for _, bad := range []string{"", "-3", "soon"} {
		_, ok := parseRetryAfter(bad, now)
		assert.Equal(t, false, ok)
	}
}

func TestRoundtrip_RetryAfter503ThenOK(t *testing.T) {
	var calls atomic.Int32
	respBytes := []byte("recovered-body")
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0") // immediate retry, keeps the test fast
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", contentTypeSSZ)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBytes)
	})

	out := &stubSSZ{}
	err := c.SSZRequest(context.Background(), http.MethodGet, "/amsterdam/payloads/0x01", nil, nil, out)
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
	assert.DeepEqual(t, respBytes, out.data)
}

func TestRoundtrip_RetryAfter503Exhausts(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	err := c.SSZRequest(context.Background(), http.MethodGet, "/amsterdam/payloads/0x01", nil, nil, &stubSSZ{})
	require.NotNil(t, err)
	var apiErr *Error
	require.Equal(t, true, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.Status)
	assert.Equal(t, int32(maxRetriesOn503+1), calls.Load()) // first attempt + retries
}

func TestRoundtrip_503NoRetryAfter(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable) // no Retry-After -> no retry
	})

	err := c.SSZRequest(context.Background(), http.MethodGet, "/amsterdam/payloads/0x01", nil, nil, &stubSSZ{})
	require.NotNil(t, err)
	var apiErr *Error
	require.Equal(t, true, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.Status)
	assert.Equal(t, int32(1), calls.Load())
}

func TestRoundtrip_503RetryAfterTooLong(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "3600") // far beyond the default cap -> no retry
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	err := c.SSZRequest(context.Background(), http.MethodGet, "/amsterdam/payloads/0x01", nil, nil, &stubSSZ{})
	require.NotNil(t, err)
	assert.Equal(t, int32(1), calls.Load())
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
