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

// TestWriteBlockFetchError tests the WriteBlockFetchError function
// to ensure that the correct error message and code are written to the response
// and that the function returns the correct boolean value.
func TestWriteBlockFetchError(t *testing.T) {
	cases := []struct {
		name            string
		err             error
		expectedMessage string
		expectedCode    int
		expectedReturn  bool
	}{
		{
			name:            "BlockNotFoundError should return 404",
			err:             lookup.NewBlockNotFoundError("block not found at slot 123"),
			expectedMessage: "Block not found",
			expectedCode:    http.StatusNotFound,
			expectedReturn:  false,
		},
		{
			name:            "BlockIdParseError should return 400",
			err:             &lookup.BlockIdParseError{},
			expectedMessage: "Invalid block ID",
			expectedCode:    http.StatusBadRequest,
			expectedReturn:  false,
		},
		{
			name:            "Generic error should return 500",
			err:             errors.New("database connection failed"),
			expectedMessage: "Could not get block from block ID",
			expectedCode:    http.StatusInternalServerError,
			expectedReturn:  false,
		},
		{
			name:            "Nil block should return 404",
			err:             nil,
			expectedMessage: "Could not find requested block",
			expectedCode:    http.StatusNotFound,
			expectedReturn:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			result := WriteBlockFetchError(writer, nil, c.err)

			assert.Equal(t, c.expectedReturn, result, "incorrect return value")
			assert.Equal(t, c.expectedCode, writer.Code, "incorrect status code")
			assert.StringContains(t, c.expectedMessage, writer.Body.String(), "incorrect error message")

			e := &httputil.DefaultJsonError{}
			assert.NoError(t, json.Unmarshal(writer.Body.Bytes(), e), "failed to unmarshal response")
		})
	}
}
