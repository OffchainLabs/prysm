package apiutil

import (
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api/server/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Endpoint struct {
	Template   string
	Name       string
	Middleware []middleware.Middleware
	Handler    http.HandlerFunc
	Methods    []string
}

func (e *Endpoint) HandlerWithMiddleware() http.HandlerFunc {
	handler := http.Handler(e.Handler)
	for _, m := range e.Middleware {
		handler = m(handler)
	}

	handler = promhttp.InstrumentHandlerDuration(
		httpRequestLatency.MustCurryWith(prometheus.Labels{"endpoint": e.Name}),
		promhttp.InstrumentHandlerCounter(
			httpRequestCount.MustCurryWith(prometheus.Labels{"endpoint": e.Name}),
			handler,
		),
	)

	return func(w http.ResponseWriter, r *http.Request) {
		// SSE errors are handled separately to avoid interference with the streaming
		// mechanism and ensure accurate error tracking.
		if e.Template == "/eth/v1/events" {
			handler.ServeHTTP(w, r)
			return
		}

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		handler.ServeHTTP(rw, r)

		if rw.statusCode >= 400 {
			httpErrorCount.WithLabelValues(r.URL.Path, http.StatusText(rw.statusCode), r.Method).Inc()
		}
	}
}

// responseWriter is the wrapper to http Response writer.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader wraps the WriteHeader method of the underlying http.ResponseWriter to capture the status code.
// Refer for WriteHeader doc: https://pkg.go.dev/net/http@go1.23.3#ResponseWriter.
func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}
