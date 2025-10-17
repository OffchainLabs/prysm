package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

func TestGet(t *testing.T) {
	ctx := t.Context()
	const endpoint = "/example/rest/api/endpoint"
	genesisJson := &structs.GetGenesisResponse{
		Data: &structs.Genesis{
			GenesisTime:           "123",
			GenesisValidatorsRoot: "0x456",
			GenesisForkVersion:    "0x789",
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
		marshalledJson, err := json.Marshal(genesisJson)
		require.NoError(t, err)
		assert.Equal(t, version.BuildData(), r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", api.JsonMediaType)
		_, err = w.Write(marshalledJson)
		require.NoError(t, err)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	jsonRestHandler := BeaconApiRestHandler{
		client: http.Client{Timeout: time.Second * 5},
		host:   server.URL,
	}
	resp := &structs.GetGenesisResponse{}
	require.NoError(t, jsonRestHandler.Get(ctx, endpoint+"?arg1=abc&arg2=def", resp))
	assert.DeepEqual(t, genesisJson, resp)
}

func TestGetSSZ(t *testing.T) {
	ctx := context.Background()
	const endpoint = "/example/rest/api/ssz"
	genesisJson := &structs.GetGenesisResponse{
		Data: &structs.Genesis{
			GenesisTime:           "123",
			GenesisValidatorsRoot: "0x456",
			GenesisForkVersion:    "0x789",
		},
	}

	t.Run("Successful SSZ response", func(t *testing.T) {
		expectedBody := []byte{10, 20, 30, 40}

		mux := http.NewServeMux()
		mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			assert.StringContains(t, api.OctetStreamMediaType, r.Header.Get("Accept"))
			assert.Equal(t, version.BuildData(), r.Header.Get("User-Agent"))
			w.Header().Set("Content-Type", api.OctetStreamMediaType)
			_, err := w.Write(expectedBody)
			require.NoError(t, err)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		jsonRestHandler := BeaconApiRestHandler{
			client: http.Client{Timeout: time.Second * 5},
			host:   server.URL,
		}

		body, header, err := jsonRestHandler.GetSSZ(ctx, endpoint)
		require.NoError(t, err)
		assert.DeepEqual(t, expectedBody, body)
		require.StringContains(t, api.OctetStreamMediaType, header.Get("Content-Type"))
	})

	t.Run("Json Content-Type response", func(t *testing.T) {
		logrus.SetLevel(logrus.DebugLevel)
		defer logrus.SetLevel(logrus.InfoLevel) // reset it afterwards
		logHook := test.NewGlobal()
		mux := http.NewServeMux()
		mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			assert.StringContains(t, api.OctetStreamMediaType, r.Header.Get("Accept"))
			w.Header().Set("Content-Type", api.JsonMediaType)

			marshalledJson, err := json.Marshal(genesisJson)
			require.NoError(t, err)

			_, err = w.Write(marshalledJson)
			require.NoError(t, err)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		jsonRestHandler := BeaconApiRestHandler{
			client: http.Client{Timeout: time.Second * 5},
			host:   server.URL,
		}

		body, header, err := jsonRestHandler.GetSSZ(ctx, endpoint)
		require.NoError(t, err)
		assert.LogsContain(t, logHook, "Server responded with non primary accept type")
		require.Equal(t, api.JsonMediaType, header.Get("Content-Type"))
		resp := &structs.GetGenesisResponse{}
		require.NoError(t, json.Unmarshal(body, resp))
		require.Equal(t, "123", resp.Data.GenesisTime)
	})

	t.Run("Wrong Content-Type response, doesn't error out and instead handled downstream", func(t *testing.T) {
		logrus.SetLevel(logrus.DebugLevel)
		defer logrus.SetLevel(logrus.InfoLevel) // reset it afterwards
		logHook := test.NewGlobal()
		mux := http.NewServeMux()
		mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			assert.StringContains(t, api.OctetStreamMediaType, r.Header.Get("Accept"))
			w.Header().Set("Content-Type", "text/plain") // Invalid content type
			_, err := w.Write([]byte("some text"))
			require.NoError(t, err)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		jsonRestHandler := BeaconApiRestHandler{
			client: http.Client{Timeout: time.Second * 5},
			host:   server.URL,
		}

		_, _, err := jsonRestHandler.GetSSZ(ctx, endpoint)
		require.NoError(t, err)
		assert.LogsContain(t, logHook, "Server responded with non primary accept type")
	})
}

func TestAcceptOverrideSSZ(t *testing.T) {
	name := "TestAcceptOverride"
	orig := os.Getenv(params.EnvNameOverrideAccept)
	defer func() {
		require.NoError(t, os.Setenv(params.EnvNameOverrideAccept, orig))
	}()
	require.NoError(t, os.Setenv(params.EnvNameOverrideAccept, name))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, name, r.Header.Get("Accept"))
		w.WriteHeader(200)
		_, err := w.Write([]byte("ok"))
		require.NoError(t, err)
	}))
	defer srv.Close()
	c := NewBeaconApiRestHandler(http.Client{Timeout: time.Second * 5}, srv.URL)
	_, _, err := c.GetSSZ(t.Context(), "/test")
	require.NoError(t, err)
}

func TestPost(t *testing.T) {
	ctx := t.Context()
	const endpoint = "/example/rest/api/endpoint"
	dataBytes := []byte{1, 2, 3, 4, 5}
	headers := map[string]string{"foo": "bar"}

	genesisJson := &structs.GetGenesisResponse{
		Data: &structs.Genesis{
			GenesisTime:           "123",
			GenesisValidatorsRoot: "0x456",
			GenesisForkVersion:    "0x789",
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
		// Make sure the request headers have been set
		assert.Equal(t, "bar", r.Header.Get("foo"))
		assert.Equal(t, version.BuildData(), r.Header.Get("User-Agent"))
		assert.Equal(t, api.JsonMediaType, r.Header.Get("Content-Type"))

		// Make sure the data matches
		receivedBytes := make([]byte, len(dataBytes))
		numBytes, err := r.Body.Read(receivedBytes)
		assert.Equal(t, io.EOF, err)
		assert.Equal(t, len(dataBytes), numBytes)
		assert.DeepEqual(t, dataBytes, receivedBytes)

		marshalledJson, err := json.Marshal(genesisJson)
		require.NoError(t, err)

		w.Header().Set("Content-Type", api.JsonMediaType)
		_, err = w.Write(marshalledJson)
		require.NoError(t, err)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	jsonRestHandler := BeaconApiRestHandler{
		client: http.Client{Timeout: time.Second * 5},
		host:   server.URL,
	}
	resp := &structs.GetGenesisResponse{}
	require.NoError(t, jsonRestHandler.Post(ctx, endpoint, headers, bytes.NewBuffer(dataBytes), resp))
	assert.DeepEqual(t, genesisJson, resp)
}

func Test_decodeResp(t *testing.T) {
	type j struct {
		Foo string `json:"foo"`
	}
	t.Run("200 JSON with charset", func(t *testing.T) {
		body := bytes.Buffer{}
		r := &http.Response{
			Status:     "200",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {"application/json; charset=utf-8"}},
		}
		require.NoError(t, decodeResp(r, nil))
	})
	t.Run("200 non-JSON", func(t *testing.T) {
		body := bytes.Buffer{}
		r := &http.Response{
			Status:     "200",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {api.OctetStreamMediaType}},
		}
		require.NoError(t, decodeResp(r, nil))
	})
	t.Run("204 non-JSON", func(t *testing.T) {
		body := bytes.Buffer{}
		r := &http.Response{
			Status:     "204",
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {api.OctetStreamMediaType}},
		}
		require.NoError(t, decodeResp(r, nil))
	})
	t.Run("500 non-JSON", func(t *testing.T) {
		body := bytes.Buffer{}
		_, err := body.WriteString("foo")
		require.NoError(t, err)
		r := &http.Response{
			Status:     "500",
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {api.OctetStreamMediaType}},
		}
		err = decodeResp(r, nil)
		errJson := &httputil.DefaultJsonError{}
		require.Equal(t, true, errors.As(err, &errJson))
		assert.Equal(t, http.StatusInternalServerError, errJson.Code)
		assert.Equal(t, "foo", errJson.Message)
	})
	t.Run("200 JSON with resp", func(t *testing.T) {
		body := bytes.Buffer{}
		b, err := json.Marshal(&j{Foo: "foo"})
		require.NoError(t, err)
		body.Write(b)
		r := &http.Response{
			Status:     "200",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {api.JsonMediaType}},
		}
		resp := &j{}
		require.NoError(t, decodeResp(r, resp))
		assert.Equal(t, "foo", resp.Foo)
	})
	t.Run("200 JSON without resp", func(t *testing.T) {
		body := bytes.Buffer{}
		r := &http.Response{
			Status:     "200",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {api.JsonMediaType}},
		}
		require.NoError(t, decodeResp(r, nil))
	})
	t.Run("204 JSON", func(t *testing.T) {
		body := bytes.Buffer{}
		r := &http.Response{
			Status:     "204",
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {api.JsonMediaType}},
		}
		require.NoError(t, decodeResp(r, nil))
	})
	t.Run("500 JSON", func(t *testing.T) {
		body := bytes.Buffer{}
		b, err := json.Marshal(&httputil.DefaultJsonError{Code: http.StatusInternalServerError, Message: "error"})
		require.NoError(t, err)
		body.Write(b)
		r := &http.Response{
			Status:     "500",
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {api.JsonMediaType}},
		}
		err = decodeResp(r, nil)
		errJson := &httputil.DefaultJsonError{}
		require.Equal(t, true, errors.As(err, &errJson))
		assert.Equal(t, http.StatusInternalServerError, errJson.Code)
		assert.Equal(t, "error", errJson.Message)
	})
	t.Run("200 JSON cannot decode", func(t *testing.T) {
		body := bytes.Buffer{}
		_, err := body.WriteString("foo")
		require.NoError(t, err)
		r := &http.Response{
			Status:     "200",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {api.JsonMediaType}},
			Request:    &http.Request{},
		}
		resp := &j{}
		err = decodeResp(r, resp)
		assert.ErrorContains(t, "failed to decode response body into json", err)
	})
	t.Run("500 JSON cannot decode", func(t *testing.T) {
		body := bytes.Buffer{}
		_, err := body.WriteString("foo")
		require.NoError(t, err)
		r := &http.Response{
			Status:     "500",
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {api.JsonMediaType}},
			Request:    &http.Request{},
		}
		err = decodeResp(r, nil)
		assert.ErrorContains(t, "failed to decode response body into error json", err)
	})
	t.Run("500 not JSON", func(t *testing.T) {
		body := bytes.Buffer{}
		_, err := body.WriteString("foo")
		require.NoError(t, err)
		r := &http.Response{
			Status:     "500",
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(&body),
			Header:     map[string][]string{"Content-Type": {"text/plain"}},
			Request:    &http.Request{},
		}
		err = decodeResp(r, nil)
		assert.ErrorContains(t, "HTTP request unsuccessful (500: foo)", err)
	})
}
