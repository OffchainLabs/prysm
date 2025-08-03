package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/pkg/errors"
)

// TestWriteStateFetchError tests the WriteStateFetchError function
// to ensure that the correct error message and code are written to the response
// as an expected JSON format.
func TestWriteStateFetchError(t *testing.T) {
	cases := []struct {
		err             error
		expectedMessage string
		expectedCode    int
	}{
		{
			err:             &lookup.StateNotFoundError{},
			expectedMessage: "State not found",
			expectedCode:    http.StatusNotFound,
		},
		{
			err:             &lookup.StateIdParseError{},
			expectedMessage: "Invalid state ID",
			expectedCode:    http.StatusBadRequest,
		},
		{
			err:             errors.New("state not found"),
			expectedMessage: "Could not get state",
			expectedCode:    http.StatusInternalServerError,
		},
	}

	for _, c := range cases {
		writer := httptest.NewRecorder()
		WriteStateFetchError(writer, c.err)

		assert.Equal(t, c.expectedCode, writer.Code, "incorrect status code")
		assert.StringContains(t, c.expectedMessage, writer.Body.String(), "incorrect error message")

		e := &httputil.DefaultJsonError{}
		assert.NoError(t, json.Unmarshal(writer.Body.Bytes(), e), "failed to unmarshal response")
	}
}

// TestWriteStateFetchError tests the WriteStateFetchError function
// to ensure that the correct error message and code are written to the response
// as an expected JSON format.
func TestWriteOptimisticStatusError(t *testing.T) {
	cases := []struct {
		err             error
		expectedMessage string
		expectedCode    int
	}{
		{
			err:             &lookup.StateIdParseError{},
			expectedMessage: "Invalid state ID",
			expectedCode:    http.StatusBadRequest,
		},
		{
			err:             errors.New("could not fetch state"),
			expectedMessage: "could not fetch state",
			expectedCode:    http.StatusNotFound,
		},
		{
			err:             errors.New("no block roots returned from the database"),
			expectedMessage: "no block roots returned from the database",
			expectedCode:    http.StatusNotFound,
		},
		{
			err:             errors.New("could not get block roots for slot"),
			expectedMessage: "could not get block roots for slot",
			expectedCode:    http.StatusNotFound,
		},
		{
			err:             errors.New("could not obtain block"),
			expectedMessage: "could not obtain block",
			expectedCode:    http.StatusNotFound,
		},
		{
			err:             errors.New("could not get ancestor root"),
			expectedMessage: "could not get ancestor root",
			expectedCode:    http.StatusNotFound,
		},
		{
			err:             errors.New("internal server error"),
			expectedMessage: "internal server error",
			expectedCode:    http.StatusInternalServerError,
		},
	}

	for _, c := range cases {
		writer := httptest.NewRecorder()
		WriteOptimisticStatusError(writer, c.err)

		assert.Equal(t, c.expectedCode, writer.Code, "incorrect status code")
		assert.StringContains(t, c.expectedMessage, writer.Body.String(), "incorrect error message")

		e := &httputil.DefaultJsonError{}
		assert.NoError(t, json.Unmarshal(writer.Body.Bytes(), e), "failed to unmarshal response")
	}
}
