package client

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/pkg/errors"
)

const (
	MaxBodySize      int64 = 1 << 23 // 8MB default, WithMaxBodySize can override
	MaxBodySizeState int64 = 1 << 29 // 512MB
	MaxErrBodySize   int64 = 1 << 17 // 128KB

	envNameOverrideAccept = "PRYSM_API_OVERRIDE_ACCEPT"
)

// Client is a wrapper object around the HTTP client.
type Client struct {
	hc           *http.Client
	baseURL      *url.URL
	token        string
	maxBodySize  int64
	reqOverrides []ReqOption
}

// NewClient constructs a new client with the provided options (ex WithTimeout).
// `host` is the base host + port used to construct request urls. This value can be
// a URL string, or NewClient will assume an http endpoint if just `host:port` is used.
func NewClient(host string, opts ...ClientOpt) (*Client, error) {
	u, err := urlForHost(host)
	if err != nil {
		return nil, err
	}
	c := &Client{
		hc:          &http.Client{},
		baseURL:     u,
		maxBodySize: MaxBodySize,
	}
	for _, o := range opts {
		o(c)
	}
	c.appendAcceptOverride()
	return c, nil
}

// appendAcceptOverride enables the Accept header to be customized at runtime via an environment variable.
// This is specified as an env var because it is a niche option that prysm may use for performance testing or debugging
// bug which users are unlikely to need. Using an env var keeps the set of user-facing flags cleaner.
func (c *Client) appendAcceptOverride() {
	if accept := os.Getenv(envNameOverrideAccept); accept != "" {
		c.reqOverrides = append(c.reqOverrides, func(req *http.Request) {
			req.Header.Set("Accept", accept)
		})
	}
}

// Token returns the bearer token used for jwt authentication
func (c *Client) Token() string {
	return c.token
}

// BaseURL returns the base url of the client
func (c *Client) BaseURL() *url.URL {
	return c.baseURL
}

// Do execute the request against the http client
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	for _, o := range c.reqOverrides {
		o(req)
	}
	return c.hc.Do(req)
}

func urlForHost(h string) (*url.URL, error) {
	// try to parse as url (being permissive)
	u, err := url.Parse(h)
	if err == nil && u.Host != "" {
		return u, nil
	}
	// try to parse as host:port
	host, port, err := net.SplitHostPort(h)
	if err != nil {
		return nil, ErrMalformedHostname
	}
	return &url.URL{Host: net.JoinHostPort(host, port), Scheme: "http"}, nil
}

// NodeURL returns a human-readable string representation of the beacon node base url.
func (c *Client) NodeURL() string {
	return c.baseURL.String()
}

// Get is a generic, opinionated GET function to reduce boilerplate amongst the getters in this package.
func (c *Client) Get(ctx context.Context, path string, opts ...ReqOption) ([]byte, error) {
	u := c.baseURL.ResolveReference(&url.URL{Path: path})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	for _, o := range opts {
		o(req)
	}
	r, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = r.Body.Close()
	}()
	if r.StatusCode != http.StatusOK {
		return nil, Non200Err(r)
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, c.maxBodySize))
	if err != nil {
		return nil, errors.Wrap(err, "error reading http response body")
	}
	return b, nil
}
