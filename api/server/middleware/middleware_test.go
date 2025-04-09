package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestNormalizeQueryValuesHandler(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("next handler"))
		require.NoError(t, err)
	})

	handler := NormalizeQueryValuesHandler(nextHandler)

	tests := []struct {
		name          string
		inputQuery    string
		expectedQuery string
	}{
		{
			name:          "3 values",
			inputQuery:    "key=value1,value2,value3",
			expectedQuery: "key=value1&key=value2&key=value3", // replace with expected normalized value
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/test?"+test.inputQuery, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
			}

			if req.URL.RawQuery != test.expectedQuery {
				t.Errorf("query not normalized: got %v want %v", req.URL.RawQuery, test.expectedQuery)
			}

			if rr.Body.String() != "next handler" {
				t.Errorf("next handler was not executed")
			}
		})
	}
}

func TestContentTypeHandler(t *testing.T) {
	acceptedMediaTypes := []string{api.JsonMediaType, api.OctetStreamMediaType}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("next handler"))
		require.NoError(t, err)
	})

	handler := ContentTypeHandler(acceptedMediaTypes)(nextHandler)

	tests := []struct {
		name               string
		contentType        string
		expectedStatusCode int
		isGet              bool
	}{
		{
			name:               "Accepted Content-Type - application/json",
			contentType:        api.JsonMediaType,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Accepted Content-Type - ssz format",
			contentType:        api.OctetStreamMediaType,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Unsupported Content-Type - text/plain",
			contentType:        "text/plain",
			expectedStatusCode: http.StatusUnsupportedMediaType,
		},
		{
			name:               "Missing Content-Type",
			contentType:        "",
			expectedStatusCode: http.StatusUnsupportedMediaType,
		},
		{
			name:               "GET request skips content type check",
			contentType:        "",
			expectedStatusCode: http.StatusOK,
			isGet:              true,
		},
		{
			name:               "Content type contains charset is ok",
			contentType:        "application/json; charset=utf-8",
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpMethod := http.MethodPost
			if tt.isGet {
				httpMethod = http.MethodGet
			}
			req := httptest.NewRequest(httpMethod, "/", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatusCode {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatusCode)
			}
		})
	}
}

func TestAcceptHeaderHandler(t *testing.T) {
	acceptedTypes := []string{"application/json", "application/octet-stream"}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("next handler"))
		require.NoError(t, err)
	})

	handler := AcceptHeaderHandler(acceptedTypes)(nextHandler)

	tests := []struct {
		name               string
		acceptHeader       string
		expectedStatusCode int
	}{
		{
			name:               "Accepted Accept-Type - application/json",
			acceptHeader:       "application/json",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Accepted Accept-Type - application/octet-stream",
			acceptHeader:       "application/octet-stream",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Accepted Accept-Type with parameters",
			acceptHeader:       "application/json;q=0.9, application/octet-stream;q=0.8",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Unsupported Accept-Type - text/plain",
			acceptHeader:       "text/plain",
			expectedStatusCode: http.StatusNotAcceptable,
		},
		{
			name:               "Missing Accept header",
			acceptHeader:       "",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "*/* is accepted",
			acceptHeader:       "*/*",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "application/* is accepted",
			acceptHeader:       "application/*",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "/* is unsupported",
			acceptHeader:       "/*",
			expectedStatusCode: http.StatusNotAcceptable,
		},
		{
			name:               "application/ is unsupported",
			acceptHeader:       "application/",
			expectedStatusCode: http.StatusNotAcceptable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.acceptHeader != "" {
				req.Header.Set("Accept", tt.acceptHeader)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatusCode {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatusCode)
			}
		})
	}
}
