package rest

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/client"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// RestConnectionProvider manages HTTP client configuration for the REST API.
type RestConnectionProvider interface {
	// HttpClient returns the configured HTTP client with headers, timeout, and optional tracing.
	HttpClient() *http.Client
	// Handler returns the REST handler for making API requests.
	Handler() Handler
	// Hosts returns all configured REST API endpoint URLs.
	Hosts() []string
}

// RestConnectionProviderOption is a functional option for configuring the REST connection provider.
type RestConnectionProviderOption func(*restConnectionProviderConfig)

// WithHttpTimeout sets the HTTP client timeout.
func WithHttpTimeout(timeout time.Duration) RestConnectionProviderOption {
	return func(c *restConnectionProviderConfig) {
		c.timeout = timeout
	}
}

// WithHttpHeaders sets custom HTTP headers to include in all requests.
func WithHttpHeaders(headers map[string][]string) RestConnectionProviderOption {
	return func(c *restConnectionProviderConfig) {
		c.headers = headers
	}
}

// WithTracing enables OpenTelemetry tracing for HTTP requests.
func WithTracing() RestConnectionProviderOption {
	return func(c *restConnectionProviderConfig) {
		c.enableTracing = true
	}
}

type restConnectionProviderConfig struct {
	timeout       time.Duration
	headers       map[string][]string
	enableTracing bool
}

type restConnectionProvider struct {
	endpoints   []string
	httpClient  *http.Client
	restHandler Handler
}

// NewRestConnectionProvider creates a new REST connection provider that manages HTTP client configuration.
// The endpoint parameter can be a comma-separated list of URLs (e.g., "http://host1:3500,http://host2:3500").
func NewRestConnectionProvider(endpoint string, opts ...RestConnectionProviderOption) (RestConnectionProvider, error) {
	endpoints := parseEndpoints(endpoint)
	if len(endpoints) == 0 {
		return nil, errors.New("no REST API endpoints provided")
	}

	cfg := restConnectionProviderConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	// Build the HTTP transport chain
	var transport http.RoundTripper = http.DefaultTransport

	// Add custom headers if configured
	if len(cfg.headers) > 0 {
		transport = client.NewCustomHeadersTransport(transport, cfg.headers)
	}
	// Add tracing if enabled
	if cfg.enableTracing {
		transport = otelhttp.NewTransport(transport)
	}

	httpClient := &http.Client{
		Timeout:   cfg.timeout,
		Transport: transport,
	}

	// Build one handler per endpoint and fan out reads/writes across all of them.
	handlers := make([]*handler, 0, len(endpoints))
	for _, endpoint := range endpoints {
		handlers = append(handlers, newHandler(*httpClient, endpoint))
	}

	restHandler, err := newMultiHandler(handlers)
	if err != nil {
		return nil, fmt.Errorf("new multi handler: %w", err)
	}

	p := &restConnectionProvider{
		endpoints:   endpoints,
		httpClient:  httpClient,
		restHandler: restHandler,
	}

	log.WithFields(logrus.Fields{
		"endpoints": api.RedactEndpoints(endpoints),
		"count":     len(endpoints),
	}).Info("Initialized REST connection provider")

	return p, nil
}

// parseEndpoints splits a comma-separated endpoint string into individual endpoints.
func parseEndpoints(endpoint string) []string {
	if endpoint == "" {
		return nil
	}
	endpoints := make([]string, 0, 1)
	for p := range strings.SplitSeq(endpoint, ",") {
		if p = strings.TrimSpace(p); p != "" {
			endpoints = append(endpoints, p)
		}
	}
	return endpoints
}

func (p *restConnectionProvider) HttpClient() *http.Client {
	return p.httpClient
}

func (p *restConnectionProvider) Handler() Handler {
	return p.restHandler
}

func (p *restConnectionProvider) Hosts() []string {
	// Return a copy to maintain immutability
	hosts := make([]string, len(p.endpoints))
	copy(hosts, p.endpoints)
	return hosts
}
