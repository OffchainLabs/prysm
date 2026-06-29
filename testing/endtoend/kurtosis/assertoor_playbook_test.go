package kurtosis

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterAndScheduleTest(t *testing.T) {
	var registeredBody string
	var scheduledTestID string
	var scheduledSkipQueue bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tests/register", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		registeredBody = string(b)
		_, _ = w.Write([]byte(`{"status":"OK","data":{"test_id":"validators-active","name":"x","config":{}}}`))
	})
	mux.HandleFunc("/api/v1/test_runs/schedule", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TestID    string `json:"test_id"`
			SkipQueue bool   `json:"skip_queue"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		scheduledTestID = req.TestID
		scheduledSkipQueue = req.SkipQueue
		_, _ = w.Write([]byte(`{"status":"OK","data":{"test_id":"validators-active","run_id":1,"name":"x"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	yaml := []byte("id: validators-active\nname: x\ntasks: []\n")
	if err := registerAndScheduleTest(context.Background(), srv.URL, yaml); err != nil {
		t.Fatalf("registerAndScheduleTest: %v", err)
	}
	// The YAML must be POSTed verbatim, and the returned test_id scheduled off-queue.
	if registeredBody != string(yaml) {
		t.Fatalf("register body mismatch: got %q", registeredBody)
	}
	if scheduledTestID != "validators-active" {
		t.Fatalf("scheduled wrong test id: got %q", scheduledTestID)
	}
	if !scheduledSkipQueue {
		t.Fatal("expected skip_queue=true so custom tests run in parallel")
	}
}
