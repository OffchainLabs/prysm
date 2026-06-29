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

const (
	contentTypeJSON = "application/json"
	contentTypeYAML = "application/yaml"
)

// WaitForAssertoor polls every Assertoor test run until all are terminal, then
// returns an error unless every run succeeded, naming the failing tests and tasks.
func (kw *KurtosisWrapper) WaitForAssertoor(ctx context.Context, deadline time.Time) error {
	baseURL, err := kw.NewAssertoorEndpoint()
	if err != nil {
		return err
	}
	return waitForAssertoorRuns(ctx, baseURL, deadline, 5*time.Second)
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
		if r.Status != "failure" && r.Status != "aborted" {
			continue
		}
		var detail assertoorRunDetail
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

// waitForAssertoorRuns polls baseURL until a run fails (fail fast) or every run
// finishes, then renders a verdict.
func waitForAssertoorRuns(ctx context.Context, baseURL string, deadline time.Time, pollInterval time.Duration) error {
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var runs []assertoorRun
	for {
		// Errors here (Assertoor not up yet, runs not scheduled yet) are transient: retry.
		if got, err := assertoorRuns(ctx, baseURL); err == nil && len(got) > 0 {
			runs = got
			switch {
			case hasFailure(runs):
				return assertoorVerdict(ctx, baseURL, runs)
			case allTerminal(runs):
				return nil // every run finished, none failed
			}
		}

		select {
		case <-ctx.Done():
			if len(runs) == 0 {
				return fmt.Errorf("timed out waiting for Assertoor test runs: %w", ctx.Err())
			}
			return nil // deadline reached with no failure: monitors ran the full window clean
		case <-time.After(pollInterval):
		}
	}
}

func allTerminal(runs []assertoorRun) bool {
	for _, r := range runs {
		if !r.terminal() {
			return false
		}
	}
	return true
}

func hasFailure(runs []assertoorRun) bool {
	for _, r := range runs {
		if r.Status == "failure" || r.Status == "aborted" {
			return true
		}
	}
	return false
}

// assertoorVerdict returns nil if every run succeeded, else an error naming each
// failing test (and its failed tasks, fetched from the run detail).
func assertoorVerdict(ctx context.Context, baseURL string, runs []assertoorRun) error {
	var failures []string
	for _, r := range runs {
		if r.Status != "failure" && r.Status != "aborted" {
			continue // success, skipped, or still-running monitors aren't failures
		}

		msg := fmt.Sprintf("%s [%s]", r.label(), r.Status)

		var detail assertoorRun
		if err := assertoorGet(ctx, fmt.Sprintf("%s/api/v1/test_run/%d", baseURL, r.RunID), &detail); err == nil {
			if tasks := detail.failedTasks(); len(tasks) > 0 {
				msg += ": " + strings.Join(tasks, "; ")
			}
		}
		failures = append(failures, msg)
	}

	if len(failures) > 0 {
		return fmt.Errorf("Assertoor checks failed: %s", strings.Join(failures, " | "))
	}
	return nil
}

// assertoorRuns lists all test runs known to Assertoor.
func assertoorRuns(ctx context.Context, baseURL string) ([]assertoorRun, error) {
	var runs []assertoorRun
	if err := assertoorGet(ctx, baseURL+"/api/v1/test_runs", &runs); err != nil {
		return nil, err
	}
	return runs, nil
}

// assertoorGet GETs an Assertoor API endpoint and unmarshals its data field into out.
func assertoorGet(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
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
		return err
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
