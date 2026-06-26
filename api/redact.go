package api

import "net/url"

// RedactEndpoint returns a loggable form of a beacon node endpoint with any
// basic-auth credentials masked. If the endpoint cannot be parsed it returns a
// placeholder rather than the raw string, so a token is never leaked to logs.
func RedactEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "[invalid endpoint]"
	}
	return u.Redacted()
}

// RedactEndpoints applies RedactEndpoint to every endpoint in the slice.
func RedactEndpoints(endpoints []string) []string {
	redacted := make([]string, len(endpoints))
	for i, e := range endpoints {
		redacted[i] = RedactEndpoint(e)
	}
	return redacted
}
