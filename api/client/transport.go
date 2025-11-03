package client

import "net/http"

// CustomHeadersTransport adds custom headers to each request
type CustomHeadersTransport struct {
	base    http.RoundTripper
	headers map[string][]string
}

func NewCustomHeadersTransport(base http.RoundTripper, headers map[string][]string) *CustomHeadersTransport {
	return &CustomHeadersTransport{
		base:    base,
		headers: headers,
	}
}

func (t *CustomHeadersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for header, values := range t.headers {
		for _, value := range values {
			req.Header.Add(header, value)
		}
	}
	return t.base.RoundTrip(req)
}
