// Package enginehttp implements an HTTP/2 (h2c) client for the REST + SSZ
// Engine API v2 (ethereum/execution-apis#793), the transport that replaces the
// JSON-RPC engine_* namespace under /engine/v2/{fork}/...
//
// The package is transport-only: it round-trips arbitrary SSZ bodies and is
// generic over the fastssz Marshaler/Unmarshaler interfaces, so it carries no
// dependency on any concrete engine container type.
package enginehttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/OffchainLabs/prysm/v7/network"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/net/http2"
)

const (
	// apiBase is the major-version-scoped base path for the v2 engine API.
	apiBase = "/engine/v2"

	// contentTypeSSZ is the hot-path body encoding (SSZ).
	contentTypeSSZ = "application/octet-stream"
	// contentTypeJSON is used by the diagnostic endpoints (/capabilities, /identity).
	contentTypeJSON = "application/json"

	// clientVersionHeader carries the CL version on every request, replacing
	// the engine_getClientVersionV1 mutual handshake.
	clientVersionHeader = "X-Engine-Client-Version"

	// defaultMaxResponseBytes is the hard ceiling on the response body read
	// from the EL, guarding against a crafted/oversized Content-Length or body
	// coercing a large allocation (execution-apis#793 "Security
	// considerations": cap the bytes read from the body in all cases). It is a
	// transport backstop set well above any legitimate engine response; the
	// tighter, semantically-correct per-endpoint limits.* caps layer on top.
	// Matches api/client's MaxBodySizeState precedent.
	defaultMaxResponseBytes int64 = 1 << 29 // 512 MiB
)

// Config configures a Client.
type Config struct {
	// BaseURL is the EL engine endpoint root, e.g. "http://localhost:8551".
	// The /engine/v2/... path is appended by the client. Required; must be http
	// (the spec mandates h2c, leaving TLS to a reverse proxy).
	BaseURL string
	// JWTSecret is the raw HS256 shared secret bytes. Required.
	JWTSecret []byte
	// JWTID is the optional "id" JWT claim.
	JWTID string
	// ClientVersion is sent as the X-Engine-Client-Version header on every
	// request (e.g. "Prysm/v7.0.0/..."). Optional.
	ClientVersion string
	// Timeout bounds each request. Defaults to network.DefaultRPCHTTPTimeout.
	Timeout time.Duration
	// MaxResponseBytes caps the response body read from the EL. Defaults to
	// defaultMaxResponseBytes when <= 0. A transport backstop against an
	// oversized/crafted body; per-endpoint limits.* caps are enforced above it.
	MaxResponseBytes int64
}

// Client is an HTTP/2 (h2c) client for the REST + SSZ Engine API v2.
type Client struct {
	base             *url.URL
	http             *http.Client
	clientVersion    string
	maxResponseBytes int64
}

// New builds a Client speaking h2c (HTTP/2 cleartext) to cfg.BaseURL with
// per-request JWT bearer auth.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("enginehttp: empty BaseURL")
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, errors.Wrap(err, "enginehttp: invalid BaseURL")
	}
	// The spec uses h2c (cleartext) on both TCP and IPC; TLS is a reverse-proxy
	// concern. Only http is supported here.
	// TODO: IPC/UNIX-socket support
	if base.Scheme != "http" {
		return nil, errors.Errorf("enginehttp: unsupported URL scheme %q (want http)", base.Scheme)
	}
	if len(cfg.JWTSecret) == 0 {
		return nil, errors.New("enginehttp: empty JWT secret")
	}

	h2 := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, netw, addr string, _ *tls.Config) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, netw, addr)
		},
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = network.DefaultRPCHTTPTimeout
	}

	maxResp := cfg.MaxResponseBytes
	if maxResp <= 0 {
		maxResp = defaultMaxResponseBytes
	}

	rt := otelhttp.NewTransport(network.NewJWTRoundTripper(h2, cfg.JWTSecret, cfg.JWTID))
	return &Client{
		base:             base,
		http:             &http.Client{Timeout: timeout, Transport: rt},
		clientVersion:    cfg.ClientVersion,
		maxResponseBytes: maxResp,
	}, nil
}

// SSZRequest performs one hot-path engine call: it marshals req as the SSZ
// request body (when non-nil), sends it to /engine/v2{p}, and on HTTP 200
// decodes the SSZ response into resp.
//
//   - method: http.MethodPost or http.MethodGet.
//   - p:      path under /engine/v2, e.g. "/amsterdam/payloads" (no trailing slash).
//   - query:  optional URL query (nil for none).
//   - req:    SSZ request body; pass nil for no body (e.g. GET endpoints).
//   - resp:   destination for a 200 SSZ body; pass nil to discard it.
//
// It returns ErrNoContent on HTTP 204, and an *Error (carrying the status and
// decoded problem+json) on any other non-200 status.
func (c *Client) SSZRequest(ctx context.Context, method, p string, query url.Values, req ssz.Marshaler, resp ssz.Unmarshaler) error {
	var body []byte
	if req != nil {
		b, err := req.MarshalSSZ()
		if err != nil {
			return errors.Wrap(err, "enginehttp: marshal SSZ request")
		}
		body = b
	}
	status, respBody, err := c.roundtrip(ctx, request{
		method:      method,
		path:        p,
		query:       query,
		body:        body,
		contentType: contentTypeSSZ,
		accept:      contentTypeSSZ,
	})
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
		if resp == nil {
			return nil
		}
		if err := resp.UnmarshalSSZ(respBody); err != nil {
			return errors.Wrap(err, "enginehttp: decode SSZ response")
		}
		return nil
	case http.StatusNoContent:
		return ErrNoContent
	default:
		return httpError(status, respBody)
	}
}

// JSONGet performs a GET against a diagnostic endpoint (/capabilities,
// /identity) and decodes the JSON response into out. p is the path under
// /engine/v2, e.g. "/capabilities". A non-200 status yields an *Error.
func (c *Client) JSONGet(ctx context.Context, p string, out any) error {
	status, body, err := c.roundtrip(ctx, request{
		method: http.MethodGet,
		path:   p,
		accept: contentTypeJSON,
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return httpError(status, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return errors.Wrap(err, "enginehttp: decode JSON response")
	}
	return nil
}

// request describes a single HTTP call to the engine API.
type request struct {
	method      string
	path        string // path under apiBase, e.g. "/amsterdam/payloads"
	query       url.Values
	body        []byte // nil for no body
	contentType string // set on the request when body != nil
	accept      string // Accept header
}

// roundtrip performs one HTTP request and returns the status code and response
// body. It returns a non-nil error only for transport/IO failures; non-2xx
// HTTP statuses are reported via the returned status for the caller to branch on.
func (c *Client) roundtrip(ctx context.Context, r request) (int, []byte, error) {
	u := *c.base
	// path.Join cleans any accidental trailing slash, matching the spec's
	// "trailing slashes are forbidden" rule.
	u.Path = path.Join(c.base.Path, apiBase, r.path)
	if r.query != nil {
		u.RawQuery = r.query.Encode()
	}

	var bodyReader io.Reader
	if r.body != nil {
		bodyReader = bytes.NewReader(r.body)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, u.String(), bodyReader)
	if err != nil {
		return 0, nil, errors.Wrap(err, "enginehttp: build request")
	}
	if r.body != nil {
		req.Header.Set("Content-Type", r.contentType)
		req.ContentLength = int64(len(r.body))
	}
	if r.accept != "" {
		req.Header.Set("Accept", r.accept)
	}
	if c.clientVersion != "" {
		req.Header.Set(clientVersionHeader, c.clientVersion)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, errors.Wrap(err, "enginehttp: request failed")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	// Cap by Content-Length before reading when present, and cap the bytes read
	// in all cases (handles an absent or lying Content-Length) — a crafted body
	// must not coerce an unbounded allocation (execution-apis#793 Security).
	if resp.ContentLength > c.maxResponseBytes {
		return resp.StatusCode, nil, errors.Errorf("enginehttp: response Content-Length %d exceeds max %d bytes", resp.ContentLength, c.maxResponseBytes)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponseBytes+1))
	if err != nil {
		return resp.StatusCode, nil, errors.Wrap(err, "enginehttp: read response body")
	}
	if int64(len(respBody)) > c.maxResponseBytes {
		return resp.StatusCode, nil, errors.Errorf("enginehttp: response body exceeds max %d bytes", c.maxResponseBytes)
	}
	return resp.StatusCode, respBody, nil
}
