package api

import (
	"net/url"
	"strings"
)

// RedactEndpoint returns a loggable form of a beacon node endpoint with any
// basic-auth credentials masked. If the endpoint cannot be parsed it returns a
// placeholder rather than the raw string, so a token is never leaked to logs.
func RedactEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err == nil && u.Opaque == "" {
		return u.Redacted()
	}
	// Scheme-less forms like 127.0.0.1:4000 fail to parse (or parse as opaque);
	// reparse as an authority so the host survives and credentials are still masked.
	u, err = url.Parse("//" + endpoint)
	if err != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		// Anything beyond a pure authority could smuggle credentials past Redacted.
		return "[invalid endpoint]"
	}
	return strings.TrimPrefix(u.Redacted(), "//")
}

// RedactEndpointList applies RedactEndpoint to a comma-separated list of
// endpoints, and re-joins the result. A single endpoint is redacted
// as-is.
func RedactEndpointList(endpoints string) string {
	redacted := strings.Split(endpoints, ",")
	for i, endpoint := range redacted {
		redacted[i] = RedactEndpoint(strings.TrimSpace(endpoint))
	}

	return strings.Join(redacted, ",")
}

// RedactEndpoints applies RedactEndpoint to every endpoint in the slice.
func RedactEndpoints(endpoints []string) []string {
	redacted := make([]string, len(endpoints))
	for i, e := range endpoints {
		redacted[i] = RedactEndpoint(e)
	}
	return redacted
}
