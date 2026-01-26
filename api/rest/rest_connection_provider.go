package rest

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/client"
	pkgErrors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// RestConnectionProvider manages HTTP client configuration for REST API with failover support.
// It allows switching between different beacon node REST endpoints when the current one becomes unavailable.
type RestConnectionProvider interface {
	// HttpClient returns the configured HTTP client with headers, timeout, and optional tracing.
	HttpClient() *http.Client
	// RestHandler returns the REST handler for making API requests.
	RestHandler() RestHandler
	// CurrentHost returns the current REST API endpoint URL.
	CurrentHost() string
	// Hosts returns all configured REST API endpoint URLs.
	Hosts() []string
	// SwitchHost switches to the endpoint at the given index.
	SwitchHost(index int) error
}

// RestConnectionProviderOption is a functional option for configuring the REST connection provider.
type RestConnectionProviderOption func(*restConnectionProvider)

// WithHttpTimeout sets the HTTP client timeout.
func WithHttpTimeout(timeout time.Duration) RestConnectionProviderOption {
	return func(p *restConnectionProvider) {
		p.timeout = timeout
	}
}

// WithHttpHeaders sets custom HTTP headers to include in all requests.
func WithHttpHeaders(headers map[string][]string) RestConnectionProviderOption {
	return func(p *restConnectionProvider) {
		p.headers = headers
	}
}

// WithTracing enables OpenTelemetry tracing for HTTP requests.
func WithTracing() RestConnectionProviderOption {
	return func(p *restConnectionProvider) {
		p.enableTracing = true
	}
}

type restConnectionProvider struct {
	endpoints     []string
	httpClient    *http.Client
	restHandler   RestHandler
	currentIndex  atomic.Uint64
	timeout       time.Duration
	headers       map[string][]string
	enableTracing bool
}

// NewRestConnectionProvider creates a new REST connection provider that manages HTTP client configuration.
// The endpoint parameter can be a comma-separated list of URLs (e.g., "http://host1:3500,http://host2:3500").
func NewRestConnectionProvider(endpoint string, opts ...RestConnectionProviderOption) (RestConnectionProvider, error) {
	endpoints := parseEndpoints(endpoint)
	if len(endpoints) == 0 {
		return nil, pkgErrors.New("no REST API endpoints provided")
	}

	p := &restConnectionProvider{
		endpoints: endpoints,
	}

	for _, opt := range opts {
		opt(p)
	}

	// Build the HTTP transport chain
	var transport http.RoundTripper = http.DefaultTransport

	// Add custom headers if configured
	if len(p.headers) > 0 {
		transport = client.NewCustomHeadersTransport(transport, p.headers)
	}

	// Add tracing if enabled
	if p.enableTracing {
		transport = otelhttp.NewTransport(transport)
	}

	p.httpClient = &http.Client{
		Timeout:   p.timeout,
		Transport: transport,
	}

	// Create the REST handler with the HTTP client and initial host
	p.restHandler = newRestHandler(*p.httpClient, endpoints[0])

	log.WithFields(logrus.Fields{
		"endpoints": endpoints,
		"count":     len(endpoints),
	}).Info("Initialized REST connection provider")

	return p, nil
}

// parseEndpoints splits a comma-separated endpoint string into individual endpoints.
func parseEndpoints(endpoint string) []string {
	if endpoint == "" {
		return nil
	}
	var endpoints []string
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

func (p *restConnectionProvider) RestHandler() RestHandler {
	return p.restHandler
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
		return pkgErrors.Errorf("invalid host index %d, must be between 0 and %d", index, len(p.endpoints)-1)
	}

	oldIdx := p.currentIndex.Load()
	p.currentIndex.Store(uint64(index))

	// Update the rest handler's host
	p.restHandler.SetHost(p.endpoints[index])

	log.WithFields(logrus.Fields{
		"previousHost": p.endpoints[oldIdx],
		"newHost":      p.endpoints[index],
	}).Debug("Switched REST endpoint")
	return nil
}
