package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

const (
	// DefaultMaxRequestSize is the default maximum request body size (100MB)
	DefaultMaxRequestSize = 100 * 1024 * 1024
	// DefaultMaxResponseSize is the default maximum response body size (100MB)
	DefaultMaxResponseSize = 100 * 1024 * 1024
)

// PayloadLimitsConfig holds configuration for payload size limits
type PayloadLimitsConfig struct {
	MaxRequestSize  int64
	MaxResponseSize int64
}

// NewDefaultPayloadLimitsConfig returns a default configuration
func NewDefaultPayloadLimitsConfig() *PayloadLimitsConfig {
	return &PayloadLimitsConfig{
		MaxRequestSize:  DefaultMaxRequestSize,
		MaxResponseSize: DefaultMaxResponseSize,
	}
}

// PayloadLimitsHandler enforces request and response size limits
func PayloadLimitsHandler(config *PayloadLimitsConfig) Middleware {
	if config == nil {
		config = NewDefaultPayloadLimitsConfig()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check Content-Length header for request size
			if r.ContentLength > 0 && r.ContentLength > config.MaxRequestSize {
				http.Error(w, fmt.Sprintf("Request body too large. Maximum size is %d bytes", config.MaxRequestSize), http.StatusRequestEntityTooLarge)
				return
			}

			// Wrap the request body with a limited reader
			if r.Body != nil && r.Body != http.NoBody {
				r.Body = &limitedReadCloser{
					ReadCloser: r.Body,
					limit:      config.MaxRequestSize,
				}
			}

			// Wrap the response writer to limit response size
			lrw := &limitedResponseWriter{
				ResponseWriter: w,
				limit:          config.MaxResponseSize,
				written:        0,
			}

			next.ServeHTTP(lrw, r)
		})
	}
}

// limitedReadCloser wraps a ReadCloser with size limit enforcement
type limitedReadCloser struct {
	io.ReadCloser
	limit int64
	read  int64
}

func (lrc *limitedReadCloser) Read(p []byte) (n int, err error) {
	if lrc.read >= lrc.limit {
		return 0, fmt.Errorf("request body exceeds maximum size of %d bytes", lrc.limit)
	}

	remaining := lrc.limit - lrc.read
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}

	n, err = lrc.ReadCloser.Read(p)
	lrc.read += int64(n)

	if lrc.read > lrc.limit {
		return n, fmt.Errorf("request body exceeds maximum size of %d bytes", lrc.limit)
	}

	return n, err
}

// limitedResponseWriter wraps a ResponseWriter with size limit enforcement
type limitedResponseWriter struct {
	http.ResponseWriter
	limit          int64
	written        int64
	limitReached   bool
	statusCode     int
	headerWritten  bool
}

func (lrw *limitedResponseWriter) WriteHeader(statusCode int) {
	if lrw.headerWritten {
		return
	}
	lrw.statusCode = statusCode
	if !lrw.limitReached {
		lrw.ResponseWriter.WriteHeader(statusCode)
		lrw.headerWritten = true
	}
}

func (lrw *limitedResponseWriter) Write(b []byte) (int, error) {
	if lrw.limitReached {
		return len(b), nil // Pretend we wrote it to avoid breaking the handler
	}

	// Check if this write would exceed the limit
	if lrw.written + int64(len(b)) > lrw.limit {
		// Response would exceed limit
		lrw.limitReached = true
		
		// Write error response
		if !lrw.headerWritten {
			lrw.ResponseWriter.WriteHeader(http.StatusInternalServerError)
			lrw.headerWritten = true
		}
		
		errorMsg := fmt.Sprintf("Response body exceeds maximum size of %d bytes", lrw.limit)
		lrw.ResponseWriter.Write([]byte(errorMsg))
		
		return len(b), fmt.Errorf("response body exceeds maximum size of %d bytes", lrw.limit)
	}

	// Ensure header is written before body
	if !lrw.headerWritten {
		if lrw.statusCode == 0 {
			lrw.statusCode = http.StatusOK
		}
		lrw.ResponseWriter.WriteHeader(lrw.statusCode)
		lrw.headerWritten = true
	}

	n, err := lrw.ResponseWriter.Write(b)
	lrw.written += int64(n)
	return n, err
}

// CustomPayloadLimitsHandler allows per-endpoint configuration of payload limits
func CustomPayloadLimitsHandler(maxRequestSize, maxResponseSize int64) Middleware {
	return PayloadLimitsHandler(&PayloadLimitsConfig{
		MaxRequestSize:  maxRequestSize,
		MaxResponseSize: maxResponseSize,
	})
}

// RequestSizeLimitHandler enforces only request size limits
func RequestSizeLimitHandler(maxSize int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check Content-Length header
			if r.ContentLength > 0 && r.ContentLength > maxSize {
				http.Error(w, fmt.Sprintf("Request body too large. Maximum size is %d bytes", maxSize), http.StatusRequestEntityTooLarge)
				return
			}

			// Wrap the request body with a limited reader
			if r.Body != nil && r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, maxSize)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ResponseSizeLimitHandler enforces only response size limits
func ResponseSizeLimitHandler(maxSize int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lrw := &limitedResponseWriter{
				ResponseWriter: w,
				limit:          maxSize,
				written:        0,
			}

			next.ServeHTTP(lrw, r)
		})
	}
}

// BufferedPayloadLimitsHandler provides size limiting with buffering for better error handling
func BufferedPayloadLimitsHandler(config *PayloadLimitsConfig) Middleware {
	if config == nil {
		config = NewDefaultPayloadLimitsConfig()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle request size limit
			if r.Body != nil && r.Body != http.NoBody {
				// Read and buffer the request body
				body, err := io.ReadAll(io.LimitReader(r.Body, config.MaxRequestSize+1))
				r.Body.Close()
				
				if err != nil {
					http.Error(w, "Failed to read request body", http.StatusBadRequest)
					return
				}
				
				if int64(len(body)) > config.MaxRequestSize {
					http.Error(w, fmt.Sprintf("Request body too large. Maximum size is %d bytes", config.MaxRequestSize), http.StatusRequestEntityTooLarge)
					return
				}
				
				// Replace the body with a new reader
				r.Body = io.NopCloser(bytes.NewReader(body))
				r.ContentLength = int64(len(body))
			}

			// Handle response with buffering
			brw := &bufferedResponseWriter{
				ResponseWriter: w,
				limit:          config.MaxResponseSize,
				buffer:         &bytes.Buffer{},
			}

			next.ServeHTTP(brw, r)

			// Check if response exceeded limit
			if int64(brw.buffer.Len()) > config.MaxResponseSize {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "Response body exceeds maximum size of %d bytes", config.MaxResponseSize)
				return
			}

			// Write the buffered response
			if brw.statusCode != 0 {
				w.WriteHeader(brw.statusCode)
			}
			w.Write(brw.buffer.Bytes())
		})
	}
}

// bufferedResponseWriter buffers the entire response before sending
type bufferedResponseWriter struct {
	http.ResponseWriter
	limit      int64
	buffer     *bytes.Buffer
	statusCode int
}

func (brw *bufferedResponseWriter) WriteHeader(statusCode int) {
	brw.statusCode = statusCode
}

func (brw *bufferedResponseWriter) Write(b []byte) (int, error) {
	return brw.buffer.Write(b)
}