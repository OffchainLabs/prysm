package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/cors"
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
			acceptHeader := r.Header.Get("Accept")
			// header is optional and should skip if not provided
			if acceptHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			accepted := false
			acceptTypes := strings.Split(acceptHeader, ",")
			// follows rules defined in https://datatracker.ietf.org/doc/html/rfc2616#section-14.1
			for _, acceptType := range acceptTypes {
				acceptType = strings.TrimSpace(acceptType)
				if acceptType == "*/*" {
					accepted = true
					break
				}
				for _, serverAcceptedType := range serverAcceptedTypes {
					if strings.HasPrefix(acceptType, serverAcceptedType) {
						accepted = true
						break
					}
					if acceptType != "/*" && strings.HasSuffix(acceptType, "/*") && strings.HasPrefix(serverAcceptedType, acceptType[:len(acceptType)-2]) {
						accepted = true
						break
					}
				}
				if accepted {
					break
				}
			}

			if !accepted {
				http.Error(w, fmt.Sprintf("Not Acceptable: %s", acceptHeader), http.StatusNotAcceptable)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
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
