package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPayloadLimitsHandler(t *testing.T) {
	tests := []struct {
		name               string
		config             *PayloadLimitsConfig
		requestBody        string
		responseBody       string
		expectedStatusCode int
		expectError        bool
	}{
		{
			name: "request within limits",
			config: &PayloadLimitsConfig{
				MaxRequestSize:  100,
				MaxResponseSize: 100,
			},
			requestBody:        "small request",
			responseBody:       "small response",
			expectedStatusCode: http.StatusOK,
			expectError:        false,
		},
		{
			name: "request exceeds limit via Content-Length",
			config: &PayloadLimitsConfig{
				MaxRequestSize:  10,
				MaxResponseSize: 100,
			},
			requestBody:        "this request body is too large",
			expectedStatusCode: http.StatusRequestEntityTooLarge,
			expectError:        true,
		},
		{
			name: "response exceeds limit",
			config: &PayloadLimitsConfig{
				MaxRequestSize:  100,
				MaxResponseSize: 10,
			},
			requestBody:        "ok",
			responseBody:       "this response body is way too large for the configured limit",
			expectedStatusCode: http.StatusInternalServerError,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Echo the request body if needed
				if r.Body != nil {
					body, _ := io.ReadAll(r.Body)
					_ = body // Just consume it
				}
				
				if tt.responseBody != "" {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(tt.responseBody))
				}
			})

			middleware := PayloadLimitsHandler(tt.config)
			wrappedHandler := middleware(handler)

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.requestBody))
			req.ContentLength = int64(len(tt.requestBody))
			rec := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rec, req)

			if tt.expectError {
				if rec.Code != tt.expectedStatusCode {
					t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rec.Code)
				}
			} else {
				if rec.Code != tt.expectedStatusCode {
					t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rec.Code)
				}
				if tt.responseBody != "" && rec.Body.String() != tt.responseBody {
					t.Errorf("expected response body %q, got %q", tt.responseBody, rec.Body.String())
				}
			}
		})
	}
}

func TestRequestSizeLimitHandler(t *testing.T) {
	tests := []struct {
		name               string
		maxSize            int64
		requestBody        string
		expectedStatusCode int
	}{
		{
			name:               "request within limit",
			maxSize:            100,
			requestBody:        "small request",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "request exceeds limit",
			maxSize:            10,
			requestBody:        "this request is too large",
			expectedStatusCode: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
				w.Write(body)
			})

			middleware := RequestSizeLimitHandler(tt.maxSize)
			wrappedHandler := middleware(handler)

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.requestBody))
			req.ContentLength = int64(len(tt.requestBody))
			rec := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rec.Code)
			}
		})
	}
}

func TestResponseSizeLimitHandler(t *testing.T) {
	tests := []struct {
		name               string
		maxSize            int64
		responseBody       string
		expectedStatusCode int
		expectTruncated    bool
	}{
		{
			name:               "response within limit",
			maxSize:            100,
			responseBody:       "small response",
			expectedStatusCode: http.StatusOK,
			expectTruncated:    false,
		},
		{
			name:               "response exceeds limit",
			maxSize:            10,
			responseBody:       "this response is way too large for the limit",
			expectedStatusCode: http.StatusInternalServerError,
			expectTruncated:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.responseBody))
			})

			middleware := ResponseSizeLimitHandler(tt.maxSize)
			wrappedHandler := middleware(handler)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rec, req)

			if tt.expectTruncated {
				if rec.Code != tt.expectedStatusCode {
					t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rec.Code)
				}
			} else {
				if rec.Code != tt.expectedStatusCode {
					t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rec.Code)
				}
				if rec.Body.String() != tt.responseBody {
					t.Errorf("expected response body %q, got %q", tt.responseBody, rec.Body.String())
				}
			}
		})
	}
}

func TestBufferedPayloadLimitsHandler(t *testing.T) {
	tests := []struct {
		name               string
		config             *PayloadLimitsConfig
		requestBody        string
		responseBody       string
		expectedStatusCode int
		expectError        bool
	}{
		{
			name: "both within limits",
			config: &PayloadLimitsConfig{
				MaxRequestSize:  100,
				MaxResponseSize: 100,
			},
			requestBody:        "small request",
			responseBody:       "small response",
			expectedStatusCode: http.StatusOK,
			expectError:        false,
		},
		{
			name: "request exceeds limit",
			config: &PayloadLimitsConfig{
				MaxRequestSize:  10,
				MaxResponseSize: 100,
			},
			requestBody:        "this request body is too large",
			expectedStatusCode: http.StatusRequestEntityTooLarge,
			expectError:        true,
		},
		{
			name: "response exceeds limit",
			config: &PayloadLimitsConfig{
				MaxRequestSize:  100,
				MaxResponseSize: 10,
			},
			requestBody:        "ok",
			responseBody:       "this response body is way too large",
			expectedStatusCode: http.StatusInternalServerError,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = body
				
				if tt.responseBody != "" {
					w.Write([]byte(tt.responseBody))
				}
			})

			middleware := BufferedPayloadLimitsHandler(tt.config)
			wrappedHandler := middleware(handler)

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.requestBody))
			rec := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rec.Code)
			}

			if !tt.expectError && tt.responseBody != "" {
				if rec.Body.String() != tt.responseBody {
					t.Errorf("expected response body %q, got %q", tt.responseBody, rec.Body.String())
				}
			}
		})
	}
}

func TestNewDefaultPayloadLimitsConfig(t *testing.T) {
	config := NewDefaultPayloadLimitsConfig()
	
	if config.MaxRequestSize != DefaultMaxRequestSize {
		t.Errorf("expected default max request size %d, got %d", DefaultMaxRequestSize, config.MaxRequestSize)
	}
	
	if config.MaxResponseSize != DefaultMaxResponseSize {
		t.Errorf("expected default max response size %d, got %d", DefaultMaxResponseSize, config.MaxResponseSize)
	}
}

func TestCustomPayloadLimitsHandler(t *testing.T) {
	maxRequest := int64(50)
	maxResponse := int64(75)
	
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	})
	
	middleware := CustomPayloadLimitsHandler(maxRequest, maxResponse)
	wrappedHandler := middleware(handler)
	
	// Test with small request
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte("test")))
	rec := httptest.NewRecorder()
	
	wrappedHandler.ServeHTTP(rec, req)
	
	if rec.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, rec.Code)
	}
	
	// Test with large request
	largeBody := bytes.Repeat([]byte("a"), int(maxRequest)+10)
	req = httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(largeBody))
	req.ContentLength = int64(len(largeBody))
	rec = httptest.NewRecorder()
	
	wrappedHandler.ServeHTTP(rec, req)
	
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status code %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}
}