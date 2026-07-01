package kurtosis

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterAssertoorTest(t *testing.T) {
	var registeredBody string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tests/register", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		registeredBody = string(b)
		_, _ = w.Write([]byte(`{"status":"OK","data":{"test_id":"validators-active","name":"x","config":{}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	yaml := []byte("id: validators-active\nname: x\ntasks: []\n")
	testID, err := registerAssertoorTest(context.Background(), srv.URL, yaml)
	if err != nil {
		t.Fatalf("registerAssertoorTest: %v", err)
	}
	if registeredBody != string(yaml) {
		t.Fatalf("register body mismatch: got %q", registeredBody)
	}
	if testID != "validators-active" {
		t.Fatalf("registered wrong test id: got %q", testID)
	}
}

func TestScheduleAssertoorTestWithConfig(t *testing.T) {
	var targetEpoch float64
	var scheduledSkipQueue bool
	var scheduledAllowDuplicate bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/test_runs/schedule", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TestID         string         `json:"test_id"`
			Config         map[string]any `json:"config"`
			SkipQueue      bool           `json:"skip_queue"`
			AllowDuplicate bool           `json:"allow_duplicate"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.TestID != "network-health-once" {
			t.Fatalf("scheduled wrong test id: got %q", req.TestID)
		}
		targetEpoch, _ = req.Config["targetEpoch"].(float64)
		scheduledSkipQueue = req.SkipQueue
		scheduledAllowDuplicate = req.AllowDuplicate
		_, _ = w.Write([]byte(`{"status":"OK","data":{"test_id":"network-health-once","run_id":42,"name":"x"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	runID, err := scheduleAssertoorTest(context.Background(), srv.URL, "network-health-once", map[string]any{"targetEpoch": 15})
	if err != nil {
		t.Fatalf("scheduleAssertoorTest: %v", err)
	}
	if runID != 42 {
		t.Fatalf("expected run id 42, got %d", runID)
	}
	if targetEpoch != 15 {
		t.Fatalf("expected targetEpoch override, got %v", targetEpoch)
	}
	if !scheduledSkipQueue {
		t.Fatal("expected skip_queue=true so custom tests run in parallel")
	}
	if !scheduledAllowDuplicate {
		t.Fatal("expected allow_duplicate=true so one-shot tests can be reused")
	}
}

func TestOneShotPlaybooksAreOptional(t *testing.T) {
	for _, name := range []string{
		"metrics-once.yaml",
		"network-health-once.yaml",
		"validators-sync-participation-once.yaml",
	} {
		if !optionalPlaybooks[name] {
			t.Fatalf("%s must not auto-run with common playbooks", name)
		}
	}
}
