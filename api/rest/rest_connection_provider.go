package rest

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/client"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// RestConnectionProvider manages HTTP client configuration for REST API with failover support.
// It allows switching between different beacon node REST endpoints when the current one becomes unavailable.
type RestConnectionProvider interface {
	// Handler returns the REST handler for making API requests.
	Handler() Handler
	// CurrentHost returns the current REST API endpoint URL.
	CurrentHost() string
	// Hosts returns all configured REST API endpoint URLs.
	Hosts() []string
	// SwitchHost switches to the endpoint at the given index.
	SwitchHost(index int) error
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
	endpoints    []string
	handler      *handler
	currentIndex atomic.Uint64
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

	httpClient := http.Client{
		Timeout:   cfg.timeout,
		Transport: transport,
	}
	p := &restConnectionProvider{
		endpoints: endpoints,
		// Create the REST handler with the HTTP client and initial host
		handler: newHandler(httpClient, endpoints[0]),
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

func (p *restConnectionProvider) Handler() Handler {
	return p.handler
}

func (p *restConnectionProvider) CurrentHost() string {
	return p.endpoints[p.currentIndex.Load()]
}

func (p *restConnectionProvider) Hosts() []string {
	// Return a copy to maintain immutability
	hosts := make([]string, len(p.endpoints))
	copy(hosts, p.endpoints)
	return hosts
}

func (p *restConnectionProvider) SwitchHost(index int) error {
	if index < 0 || index >= len(p.endpoints) {
		return errors.Errorf("invalid host index %d, must be between 0 and %d", index, len(p.endpoints)-1)
	}

	oldIdx := p.currentIndex.Load()
	p.currentIndex.Store(uint64(index))

	// Update the rest handler's host
	p.handler.SwitchHost(p.endpoints[index])

	log.WithFields(logrus.Fields{
		"previousHost": api.RedactEndpoint(p.endpoints[oldIdx]),
		"newHost":      api.RedactEndpoint(p.endpoints[index]),
	}).Debug("Switched REST endpoint")
	return nil
}
