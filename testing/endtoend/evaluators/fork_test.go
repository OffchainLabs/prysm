package evaluators

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestHeadBlockVersionAndSlot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Trimmed down to the fields the evaluators read.
		_, err := w.Write([]byte(`{"version":"fulu","execution_optimistic":false,"finalized":true,` +
			`"data":{"message":{"slot":"64","proposer_index":"1"},"signature":"0x00"}}`))
		require.NoError(t, err)
	}))
	defer srv.Close()

	gotVersion, gotSlot, err := headBlockVersionAndSlot(context.Background(), srv.URL)
	require.NoError(t, err)
	require.Equal(t, "fulu", gotVersion)
	require.Equal(t, uint64(64), uint64(gotSlot))
}

func TestHeadBlockVersionAndSlotError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte(`{"code":500,"message":"could not get head block"}`))
		require.NoError(t, err)
	}))
	defer srv.Close()

	_, _, err := headBlockVersionAndSlot(context.Background(), srv.URL)
	require.ErrorContains(t, "could not get head block (status code 500)", err)
}
