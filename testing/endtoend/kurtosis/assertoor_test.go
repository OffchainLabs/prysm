package kurtosis

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestWaitForAssertoorPassesWhenAllTerminal(t *testing.T) {
	t.Run("pass when all terminal", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/test_runs", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":"OK","data":[
				{"run_id":1,"test_id":"a","name":"a","status":"success"},
				{"run_id":2,"test_id":"b","name":"b","status":"skipped"}
			]}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		require.NoError(t, waitForAssertoorRunsMatching(context.Background(), srv.URL, time.Now().Add(time.Minute)))
	})

	t.Run("fail on unfinished run", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/test_runs", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":"OK","data":[
			{"run_id":1,"test_id":"done","name":"done","status":"success"},
			{"run_id":2,"test_id":"stuck","name":"stuck","status":"running"}
		]}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		err := waitForAssertoorRunsMatching(context.Background(), srv.URL, time.Now().Add(50*time.Millisecond))
		require.NotNil(t, err, "expected an unfinished run to fail the test")
		require.Equal(t, true, strings.Contains(err.Error(), "stuck"), "error should name the unfinished run, got: "+err.Error())
		require.Equal(t, false, strings.Contains(err.Error(), "done"), "error should not name terminal runs, got: "+err.Error())
	})
}
