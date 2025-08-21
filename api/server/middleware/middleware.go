package middleware

import (
	"compress/gzip"
	"fmt"
	"net/http"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/apiutil"
	"github.com/rs/cors"
	log "github.com/sirupsen/logrus"
)

type Middleware func(http.Handler) http.Handler

// NormalizeQueryValuesHandler normalizes an input query of "key=value1,value2,value3" to "key=value1&key=value2&key=value3"
func NormalizeQueryValuesHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		NormalizeQueryValues(query)
		r.URL.RawQuery = query.Encode()

		next.ServeHTTP(w, r)
	})
}

// CorsHandler sets the cors settings on api endpoints
func CorsHandler(allowOrigins []string) Middleware {
	c := cors.New(cors.Options{
		AllowedOrigins:   allowOrigins,
		AllowedMethods:   []string{http.MethodPost, http.MethodGet, http.MethodDelete, http.MethodOptions},
		AllowCredentials: true,
		MaxAge:           600,
		AllowedHeaders:   []string{"*"},
	})

	return c.Handler
}

// ContentTypeHandler checks request for the appropriate media types otherwise returning a http.StatusUnsupportedMediaType error
func ContentTypeHandler(acceptedMediaTypes []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// skip the GET request
			if r.Method == http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}
			contentType := r.Header.Get("Content-Type")
			if contentType == "" {
				http.Error(w, "Content-Type header is missing", http.StatusUnsupportedMediaType)
				return
			}

			accepted := false
			for _, acceptedType := range acceptedMediaTypes {
				if strings.Contains(strings.TrimSpace(contentType), strings.TrimSpace(acceptedType)) {
					accepted = true
					break
				}
			}

			if !accepted {
				http.Error(w, fmt.Sprintf("Unsupported media type: %s", contentType), http.StatusUnsupportedMediaType)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AcceptHeaderHandler checks if the client's response preference is handled
func AcceptHeaderHandler(serverAcceptedTypes []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := apiutil.Negotiate(r.Header.Get("Accept"), serverAcceptedTypes); !ok {
				http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AcceptEncodingHeaderHandler compresses the response before sending it back to the client, if gzip is supported.
func AcceptEncodingHeaderHandler() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			gz := gzip.NewWriter(w)
			gzipRW := &gzipResponseWriter{gz: gz, ResponseWriter: w}
			defer func() {
				if !gzipRW.zip {
					return
				}
				if err := gz.Close(); err != nil {
					log.WithError(err).Error("Failed to close gzip writer")
				}
			}()

			next.ServeHTTP(gzipRW, r)
		})
	}
}

type gzipResponseWriter struct {
	gz *gzip.Writer
	http.ResponseWriter
	zip bool
}

func (g *gzipResponseWriter) WriteHeader(statusCode int) {
	if strings.Contains(g.Header().Get("Content-Type"), api.JsonMediaType) {
		// Removing the current Content-Length because zipping will change it.
		g.Header().Del("Content-Length")
		g.Header().Set("Content-Encoding", "gzip")
		g.zip = true
	}

	g.ResponseWriter.WriteHeader(statusCode)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if g.zip {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func MiddlewareChain(h http.Handler, mw []Middleware) http.Handler {
	if len(mw) < 1 {
		return h
	}

	wrapped := h
	for i := len(mw) - 1; i >= 0; i-- {
		wrapped = mw[i](wrapped)
	}
	return wrapped
}
