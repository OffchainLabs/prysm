package kurtosis

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"time"
)

const (
	// Readiness check waits maximum 1 minute (30 retries * 2 seconds interval)
	MAX_READINESS_CHECK_RETRY = 30
	READINESS_CHECK_INTERVAL  = 2 * time.Second
)

//go:embed playbooks/*.yaml
var assertoorPlaybooksFS embed.FS

// RegisterPlaybooks registers and schedules the common Assertoor playbooks
// under testing/endtoend/kurtosis/playbooks/*.yaml.
func (kw *KurtosisWrapper) RegisterPlaybooks(ctx context.Context) error {
	baseURL, err := kw.NewAssertoorEndpoint()
	if err != nil {
		return fmt.Errorf("failed to get Assertoor endpoint: %w", err)
	}

	// Gate on Assertoor readiness once, then every register/schedule below is a single request.
	if err := waitForAssertoorReady(ctx, baseURL); err != nil {
		return fmt.Errorf("failed to wait for Assertoor readiness: %w", err)
	}

	entries, err := assertoorPlaybooksFS.ReadDir("playbooks")
	if err != nil {
		return fmt.Errorf("failed to read Assertoor playbooks directory: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		data, err := assertoorPlaybooksFS.ReadFile("playbooks/" + name)
		if err != nil {
			return fmt.Errorf("failed to read Assertoor playbook %q: %w", name, err)
		}

		testID, err := registerAssertoorTest(ctx, baseURL, data)
		if err != nil {
			return fmt.Errorf("failed to register Assertoor playbook %q: %w", name, err)
		}
		if _, err := scheduleAssertoorTest(ctx, baseURL, testID, nil); err != nil {
			return fmt.Errorf("failed to schedule Assertoor playbook %q: %w", name, err)
		}
	}
	return nil
}

// waitForAssertoorReady blocks until the Assertoor API responds
// (GET /api/v1/version) or ctx is done. Poll every 2 seconds for up to 30 attempts.
func waitForAssertoorReady(ctx context.Context, baseURL string) error {
	var discard json.RawMessage
	var err error
	for range MAX_READINESS_CHECK_RETRY {
		if err = assertoorGet(ctx, baseURL+"/api/v1/version", &discard); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(READINESS_CHECK_INTERVAL):
		}
	}
	return fmt.Errorf("Assertoor API never became ready: %w", err)
}

// registerAssertoorTest registers a test with Assertoor and returns its test ID.
func registerAssertoorTest(ctx context.Context, baseURL string, testYAML []byte) (string, error) {
	var reg struct {
		TestID string `json:"test_id"`
	}
	if err := assertoorPost(ctx, baseURL+"/api/v1/tests/register", "application/yaml", testYAML, &reg); err != nil {
		return "", fmt.Errorf("failed to register Assertoor test: %w", err)
	}
	return reg.TestID, nil
}

// scheduleAssertoorTest schedules a test with Assertoor and returns its run ID.
// config is used for overriding the test config (e.g., target epoch).
func scheduleAssertoorTest(ctx context.Context, baseURL, testID string, config map[string]any) (uint64, error) {
	body, err := json.Marshal(map[string]any{
		"test_id":         testID,
		"config":          config,
		"allow_duplicate": true,
		// skip_queue runs the test off-queue (in parallel).
		"skip_queue": true,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to marshal Assertoor test schedule request: %w", err)
	}
	var scheduled struct {
		RunID uint64 `json:"run_id"`
	}
	if err := assertoorPost(ctx, baseURL+"/api/v1/test_runs/schedule", "application/json", body, &scheduled); err != nil {
		return 0, fmt.Errorf("failed to schedule Assertoor test %q: %w", testID, err)
	}
	return scheduled.RunID, nil
}
