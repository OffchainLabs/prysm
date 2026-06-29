package kurtosis

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// assertoorStub serves the two endpoints the poller reads: the run list and,
// per run id, that run's detail JSON.
func assertoorStub(listJSON string, detail map[uint64]string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/test_runs", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"OK","data":` + listJSON + `}`))
	})
	mux.HandleFunc("/api/v1/test_run/", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseUint(strings.TrimPrefix(r.URL.Path, "/api/v1/test_run/"), 10, 64)
		d, ok := detail[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"OK","data":` + d + `}`))
	})
	return httptest.NewServer(mux)
}

func TestWaitForAssertoorRuns_AllSucceed(t *testing.T) {
	list := `[{"run_id":1,"test_id":"stability-check","status":"success"},
		{"run_id":2,"test_id":"block-proposal-check","status":"success"}]`
	srv := assertoorStub(list, nil)
	defer srv.Close()

	if err := waitForAssertoorRuns(context.Background(), srv.URL, time.Now().Add(2*time.Second), time.Millisecond); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestWaitForAssertoorRuns_OneFails(t *testing.T) {
	list := `[{"run_id":1,"test_id":"stability-check","name":"Check chain stability","status":"failure"},
		{"run_id":2,"test_id":"block-proposal-check","name":"Block proposals","status":"success"}]`
	detail := map[uint64]string{
		1: `{"run_id":1,"status":"failure","tasks":[
			{"title":"Check consensus chain finality","result":"failure","result_error":"not finalized"},
			{"title":"Check clients are healthy","result":"success"}]}`,
	}
	srv := assertoorStub(list, detail)
	defer srv.Close()

	err := waitForAssertoorRuns(context.Background(), srv.URL, time.Now().Add(2*time.Second), time.Millisecond)
	if err == nil {
		t.Fatal("expected failure error, got nil")
	}
	if !strings.Contains(err.Error(), "Check chain stability") ||
		!strings.Contains(err.Error(), "Check consensus chain finality (not finalized)") {
		t.Fatalf("error should name the failing test and task, got: %v", err)
	}
	if strings.Contains(err.Error(), "Block proposals") || strings.Contains(err.Error(), "Check clients are healthy") {
		t.Fatalf("error should not mention passing test/tasks, got: %v", err)
	}
}

func TestWaitForAssertoorRuns_RunningMonitorPasses(t *testing.T) {
	// A continuous monitor never goes terminal; reaching the deadline with no
	// failure means it ran the window clean, which is a pass.
	list := `[{"run_id":1,"test_id":"network-health-monitor","status":"running"}]`
	srv := assertoorStub(list, nil)
	defer srv.Close()

	if err := waitForAssertoorRuns(context.Background(), srv.URL, time.Now().Add(50*time.Millisecond), time.Millisecond); err != nil {
		t.Fatalf("expected pass for a still-running monitor, got: %v", err)
	}
}

func TestWaitForAssertoorRuns_FailsFast(t *testing.T) {
	// A failed run must abort the wait immediately, not block on the monitor
	// that's still running, and not wait out the (far-off) deadline.
	list := `[{"run_id":1,"test_id":"validators-active","name":"Validators are active","status":"failure"},
		{"run_id":2,"test_id":"network-health-monitor","name":"Network health","status":"running"}]`
	detail := map[uint64]string{
		1: `{"run_id":1,"status":"failure","tasks":[{"title":"Wait for an active validator","result":"failure","result_error":"none active"}]}`,
	}
	srv := assertoorStub(list, detail)
	defer srv.Close()

	start := time.Now()
	err := waitForAssertoorRuns(context.Background(), srv.URL, time.Now().Add(time.Hour), time.Millisecond)
	if time.Since(start) > time.Second {
		t.Fatalf("expected fail-fast, but waited %v", time.Since(start))
	}
	if err == nil || !strings.Contains(err.Error(), "Validators are active") ||
		!strings.Contains(err.Error(), "none active") {
		t.Fatalf("error should name the failed test and task, got: %v", err)
	}
	if strings.Contains(err.Error(), "Network health") {
		t.Fatalf("error should not mention the still-running monitor, got: %v", err)
	}
}
