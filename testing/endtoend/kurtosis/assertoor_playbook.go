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

// optionalPlaybooks are config-specific playbooks that only register when a
// suite explicitly opts in.
var optionalPlaybooks = map[string]bool{
	"builder.yaml": true,

	"metrics-once.yaml":                       true,
	"network-health-once.yaml":                true,
	"validators-sync-participation-once.yaml": true,
}

// RegisterPlaybooks registers and schedules the common Assertoor playbooks, plus
// any optional playbooks named in optIn minus any common playbooks named in skip.
func (kw *KurtosisWrapper) RegisterPlaybooks(ctx context.Context, optIn, skip []string) error {
	baseURL, err := kw.NewAssertoorEndpoint()
	if err != nil {
		return err
	}

	// Gate on Assertoor readiness once, then every register/schedule below is a single request.
	if err := waitForAssertoorReady(ctx, baseURL); err != nil {
		return err
	}

	optedIn := toSet(optIn)
	skipped := toSet(skip)

	entries, err := assertoorPlaybooksFS.ReadDir("playbooks")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		if optionalPlaybooks[name] && !optedIn[name] {
			continue
		}
		if skipped[name] {
			continue
		}
		data, err := assertoorPlaybooksFS.ReadFile("playbooks/" + name)
		if err != nil {
			return err
		}

		testID, err := registerAssertoorTest(ctx, baseURL, data)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if _, err := scheduleAssertoorTest(ctx, baseURL, testID, nil); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// RegisterPlaybook registers one embedded playbook by filename and returns its test ID.
func (kw *KurtosisWrapper) RegisterPlaybook(ctx context.Context, name string) (string, error) {
	baseURL, err := kw.NewAssertoorEndpoint()
	if err != nil {
		return "", err
	}
	if err := waitForAssertoorReady(ctx, baseURL); err != nil {
		return "", err
	}

	data, err := assertoorPlaybooksFS.ReadFile("playbooks/" + name)
	if err != nil {
		return "", err
	}
	return registerAssertoorTest(ctx, baseURL, data)
}

// ScheduleAssertoorTest schedules a registered test immediately and returns the run ID.
func (kw *KurtosisWrapper) ScheduleAssertoorTest(ctx context.Context, testID string, config map[string]any) (uint64, error) {
	baseURL, err := kw.NewAssertoorEndpoint()
	if err != nil {
		return 0, err
	}
	return scheduleAssertoorTest(ctx, baseURL, testID, config)
}

func toSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
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

func registerAssertoorTest(ctx context.Context, baseURL string, testYAML []byte) (string, error) {
	var reg struct {
		TestID string `json:"test_id"`
	}
	if err := assertoorPost(ctx, baseURL+"/api/v1/tests/register", contentTypeYAML, testYAML, &reg); err != nil {
		return "", fmt.Errorf("register test: %w", err)
	}
	return reg.TestID, nil
}

func scheduleAssertoorTest(ctx context.Context, baseURL, testID string, config map[string]any) (uint64, error) {
	// skip_queue runs the test off-queue (in parallel). Safe for our read-only checks.
	body, err := json.Marshal(map[string]any{
		"test_id":         testID,
		"config":          config,
		"allow_duplicate": true,
		"skip_queue":      true,
	})
	if err != nil {
		return 0, err
	}
	var scheduled struct {
		RunID uint64 `json:"run_id"`
	}
	if err := assertoorPost(ctx, baseURL+"/api/v1/test_runs/schedule", contentTypeJSON, body, &scheduled); err != nil {
		return 0, fmt.Errorf("schedule test %q: %w", testID, err)
	}
	return scheduled.RunID, nil
}
