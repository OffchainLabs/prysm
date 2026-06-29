package kurtosis

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"time"
)

//go:embed playbooks/*.yaml
var assertoorPlaybooksFS embed.FS

// RegisterPlaybooks registers and schedules every embedded custom Assertoor playbook.
func (kw *KurtosisWrapper) RegisterPlaybooks(ctx context.Context) error {
	baseURL, err := kw.NewAssertoorEndpoint()
	if err != nil {
		return err
	}

	// Gate on Assertoor readiness once, then every register/schedule below is a single request.
	if err := waitForAssertoorReady(ctx, baseURL); err != nil {
		return err
	}

	entries, err := assertoorPlaybooksFS.ReadDir("playbooks")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		data, err := assertoorPlaybooksFS.ReadFile("playbooks/" + entry.Name())
		if err != nil {
			return err
		}
		if err := registerAndScheduleTest(ctx, baseURL, data); err != nil {
			return fmt.Errorf("%s: %w", entry.Name(), err)
		}
	}
	return nil
}

// waitForAssertoorReady blocks until the Assertoor API responds
// (GET /api/v1/version) or ctx is done.
func waitForAssertoorReady(ctx context.Context, baseURL string) error {
	var discard json.RawMessage
	var err error
	for range 30 {
		if err = assertoorGet(ctx, baseURL+"/api/v1/version", &discard); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("Assertoor API never became ready: %w", err)
}

// registerAndScheduleTest registers an inline test (YAML body, with tasks) with
// Assertoor and schedules a run of it.
func registerAndScheduleTest(ctx context.Context, baseURL string, testYAML []byte) error {
	var reg struct {
		TestID string `json:"test_id"`
	}
	if err := assertoorPost(ctx, baseURL+"/api/v1/tests/register", contentTypeYAML, testYAML, &reg); err != nil {
		return fmt.Errorf("register test: %w", err)
	}

	// skip_queue runs the test off-queue (in parallel). Safe for our read-only checks.
	body, err := json.Marshal(map[string]any{"test_id": reg.TestID, "skip_queue": true})
	if err != nil {
		return err
	}
	if err := assertoorPost(ctx, baseURL+"/api/v1/test_runs/schedule", contentTypeJSON, body, nil); err != nil {
		return fmt.Errorf("schedule test %q: %w", reg.TestID, err)
	}
	return nil
}
