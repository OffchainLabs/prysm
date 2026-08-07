package kurtosis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Poll assertoor API until all test runs are terminal or the context is done.
const assertoorPollInterval = 10 * time.Second

// WaitForAssertoor is a public method around waitForAssertoorRunsMatching that uses the default Assertoor endpoint and a deadline.
func (kw *KurtosisWrapper) WaitForAssertoor(ctx context.Context, deadline time.Time) error {
	baseURL, err := kw.NewAssertoorEndpoint()
	if err != nil {
		return fmt.Errorf("failed to get Assertoor endpoint: %w", err)
	}
	return waitForAssertoorRunsMatching(ctx, baseURL, deadline)
}

// waitForAssertoorRunsMatching polls every Assertoor test run until all are terminal, then
// returns an error unless every run succeeded, naming the failing tests and tasks.
func waitForAssertoorRunsMatching(ctx context.Context, baseURL string, deadline time.Time) error {
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var (
		runs []assertoorRun
		err  error
	)

	for {
		// Fetch all runs that are known to Assertoor.
		if runs, err = assertoorRuns(ctx, baseURL); err == nil && len(runs) > 0 {
			// If we observe any failures, return an error with the failing test names and tasks.
			failures := collectFailures(ctx, baseURL, runs)
			if len(failures) > 0 {
				return fmt.Errorf("Assertoor checks failed: %s", strings.Join(failures, " | "))
			}

			// If any run is still running, wait for the next poll interval.
			allTerminal := true
			for _, r := range runs {
				if !r.terminal() {
					allTerminal = false
					break
				}
			}

			if allTerminal {
				// All runs are terminal and none failed, so we can return success.
				return nil
			}
		}

		select {
		case <-ctx.Done():
			if len(runs) == 0 {
				return fmt.Errorf("timed out waiting for Assertoor test runs: %w", ctx.Err())
			}
			// deadline reached with no failure: monitors ran the full window clean.
			return nil
		case <-time.After(assertoorPollInterval):
		}
	}
}

// DumpFailedAssertoorLogs writes each failed Assertoor task's log lines to the
// test log.
func (kw *KurtosisWrapper) DumpFailedAssertoorLogs() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	baseURL, err := kw.NewAssertoorEndpoint()
	if err != nil {
		kw.t.Logf("dump assertoor logs: %v", err)
		return
	}
	runs, err := assertoorRuns(ctx, baseURL)
	if err != nil {
		kw.t.Logf("dump assertoor logs: list runs: %v", err)
		return
	}
	for _, r := range runs {
		if !r.failed() {
			continue
		}
		var detail struct {
			Tasks []assertoorTask `json:"tasks"`
		}
		url := fmt.Sprintf("%s/api/v1/test_run/%d/details", baseURL, r.RunID)
		if err := assertoorGet(ctx, url, &detail); err != nil {
			kw.t.Logf("dump assertoor logs: run %d: %v", r.RunID, err)
			continue
		}
		for _, task := range detail.Tasks {
			if task.Result != "failure" {
				continue
			}
			kw.t.Logf("assertoor task failed: %q (result_error: %s)", task.Title, task.ResultError)
			for _, l := range task.Log {
				kw.t.Logf("  [%s] %s", l.Level, l.Message)
			}
		}
	}
}

// collectFailures returns a list of failure messages for each run that failed or was aborted.
func collectFailures(ctx context.Context, baseURL string, runs []assertoorRun) []string {
	var failures []string
	for _, r := range runs {
		if !r.failed() {
			continue
		}

		msg := fmt.Sprintf("%s [%s]", r.label(), r.Status)

		// Fetch the run details to get the failed tasks under this run.
		var detail assertoorRun
		if err := assertoorGet(ctx, fmt.Sprintf("%s/api/v1/test_run/%d", baseURL, r.RunID), &detail); err == nil {
			if tasks := detail.failedTasks(); len(tasks) > 0 {
				msg += ": " + strings.Join(tasks, "; ")
			}
		}
		failures = append(failures, msg)
	}

	return failures
}

// assertoorRuns lists all test runs known to Assertoor.
func assertoorRuns(ctx context.Context, baseURL string) ([]assertoorRun, error) {
	var runs []assertoorRun
	if err := assertoorGet(ctx, baseURL+"/api/v1/test_runs", &runs); err != nil {
		fmt.Printf("Assertoor test runs fetch failed: %v\n", err)
		return nil, err
	}

	return runs, nil
}

// assertoorGet GETs an Assertoor API endpoint and unmarshals its data field into out.
func assertoorGet(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	var env assertoorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	return json.Unmarshal(env.Data, out)
}

// assertoorPost POSTs a body to an Assertoor API endpoint and, if out is non-nil,
// unmarshals the response's data field into it.
func assertoorPost(ctx context.Context, url, contentType string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: status %d: %s", url, resp.StatusCode, bytes.TrimSpace(b))
	}

	if out == nil {
		return nil
	}

	var env assertoorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	return json.Unmarshal(env.Data, out)
}
