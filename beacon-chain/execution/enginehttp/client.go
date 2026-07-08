// Package enginehttp implements the REST + SSZ Engine API client.
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
	"strconv"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/network"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/net/http2"
)

const (
	apiBase = "/engine/v1"

	contentTypeSSZ  = "application/octet-stream"
	contentTypeJSON = "application/json"

	clientVersionHeader    = "X-Engine-Client-Version"
	executionVersionHeader = "Eth-Execution-Version"

	defaultMaxResponseBytes int64 = 1 << 29 // 512 MiB

	maxRetriesOn503 = 2

	defaultMaxRetryAfter = 2 * time.Second
)

// Config configures a Client.
type Config struct {
	// BaseURL is the EL engine endpoint root, e.g. "http://localhost:8551".
	BaseURL string
	// JWTSecret is the raw HS256 shared secret bytes. Required.
	JWTSecret []byte
	// JWTID is the optional "id" JWT claim.
	JWTID string
	// ClientVersion is sent as X-Engine-Client-Version when set.
	ClientVersion string
	// Timeout bounds each request. Defaults to network.DefaultRPCHTTPTimeout.
	Timeout time.Duration
	// MaxResponseBytes caps response bodies. Defaults when <= 0.
	MaxResponseBytes int64
	// MaxRetryAfter caps one 503 Retry-After backoff. Defaults when <= 0.
	MaxRetryAfter time.Duration
}

// Client is an HTTP/2 (h2c) client for the REST + SSZ Engine API.
type Client struct {
	base             *url.URL
	http             *http.Client
	clientVersion    string
	maxResponseBytes int64
	maxRetryAfter    time.Duration
}

// New builds an h2c client with per-request JWT bearer auth.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("enginehttp: empty BaseURL")
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, errors.Wrap(err, "enginehttp: invalid BaseURL")
	}
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

	maxRetryAfter := cfg.MaxRetryAfter
	if maxRetryAfter <= 0 {
		maxRetryAfter = defaultMaxRetryAfter
	}

	rt := otelhttp.NewTransport(network.NewJWTRoundTripper(h2, cfg.JWTSecret, cfg.JWTID))
	return &Client{
		base:             base,
		http:             &http.Client{Timeout: timeout, Transport: rt},
		clientVersion:    cfg.ClientVersion,
		maxResponseBytes: maxResp,
		maxRetryAfter:    maxRetryAfter,
	}, nil
}

// SSZRequest performs one unscoped SSZ call.
func (c *Client) SSZRequest(ctx context.Context, method, p string, query url.Values, req ssz.Marshaler, resp ssz.Unmarshaler) error {
	return c.sszRequest(ctx, method, p, query, "", req, resp)
}

// ForkSSZRequest performs one SSZ call with Eth-Execution-Version.
func (c *Client) ForkSSZRequest(ctx context.Context, method, fork, p string, query url.Values, req ssz.Marshaler, resp ssz.Unmarshaler) error {
	if fork == "" {
		return errors.New("enginehttp: empty execution version")
	}
	return c.sszRequest(ctx, method, p, query, fork, req, resp)
}

func (c *Client) sszRequest(ctx context.Context, method, p string, query url.Values, executionVersion string, req ssz.Marshaler, resp ssz.Unmarshaler) error {
	var body []byte
	if req != nil {
		b, err := req.MarshalSSZ()
		if err != nil {
			return errors.Wrap(err, "enginehttp: marshal SSZ request")
		}
		body = b
	}
	status, respBody, err := c.roundtrip(ctx, request{
		method:           method,
		path:             p,
		query:            query,
		body:             body,
		contentType:      contentTypeSSZ,
		accept:           contentTypeSSZ,
		executionVersion: executionVersion,
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

// JSONGet performs one JSON diagnostic GET.
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

type request struct {
	method           string
	path             string // path under apiBase, e.g. "/payloads" or "/blobs/v1"
	query            url.Values
	body             []byte // nil for no body
	contentType      string // set on the request when body != nil
	accept           string // Accept header
	executionVersion string // Eth-Execution-Version header, fork-scoped endpoints only
}

// roundtrip performs one engine call, retrying bounded 503 Retry-After responses.
func (c *Client) roundtrip(ctx context.Context, r request) (int, []byte, error) {
	for attempt := 0; ; attempt++ {
		status, body, retryAfter, err := c.do(ctx, r)
		if err != nil || status != http.StatusServiceUnavailable || attempt >= maxRetriesOn503 {
			return status, body, err
		}
		wait, ok := parseRetryAfter(retryAfter, time.Now())
		if !ok || wait > c.maxRetryAfter {
			return status, body, nil // no usable Retry-After (or too long): surface the 503
		}
		if err := sleepContext(ctx, wait); err != nil {
			return status, body, nil // ctx ended during backoff: surface the 503
		}
	}
}

// do performs one HTTP attempt.
func (c *Client) do(ctx context.Context, r request) (int, []byte, string, error) {
	u := *c.base
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
		return 0, nil, "", errors.Wrap(err, "enginehttp: build request")
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
	if r.executionVersion != "" {
		req.Header.Set(executionVersionHeader, r.executionVersion)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, "", errors.Wrap(err, "enginehttp: request failed")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.ContentLength > c.maxResponseBytes {
		return resp.StatusCode, nil, "", errors.Errorf("enginehttp: response Content-Length %d exceeds max %d bytes", resp.ContentLength, c.maxResponseBytes)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponseBytes+1))
	if err != nil {
		return resp.StatusCode, nil, "", errors.Wrap(err, "enginehttp: read response body")
	}
	if int64(len(respBody)) > c.maxResponseBytes {
		return resp.StatusCode, nil, "", errors.Errorf("enginehttp: response body exceeds max %d bytes", c.maxResponseBytes)
	}
	return resp.StatusCode, respBody, resp.Header.Get("Retry-After"), nil
}

// parseRetryAfter parses an RFC 7231 Retry-After value (delay-seconds or an
// HTTP-date) into a non-negative delay, reporting whether it was present and
// understood. now is the reference for the date form.
func parseRetryAfter(h string, now time.Time) (time.Duration, bool) {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(h); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := t.Sub(now); d > 0 {
			return d, true
		}
		return 0, true // a past/now date means retry immediately
	}
	return 0, false
}

// sleepContext waits for d or until ctx ends, returning ctx.Err() if ctx ended.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
