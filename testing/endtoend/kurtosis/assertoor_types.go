package kurtosis

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
)

// assertoorEnvelope is the {status, data} wrapper Assertoor puts around every API response.
type assertoorEnvelope struct {
	Data json.RawMessage `json:"data"`
}

// assertoorRun is the subset of an Assertoor test run we care about. The list
// endpoint (/test_runs) omits tasks; the detail endpoint (/test_run/{id}) fills them.
type assertoorRun struct {
	RunID  uint64 `json:"run_id"`
	TestID string `json:"test_id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Tasks  []struct {
		Title       string `json:"title"`
		Result      string `json:"result"`
		ResultError string `json:"result_error"`
	} `json:"tasks"`
}

// assertoorRunDetail is the subset of the /test_run/{id}/details response we use
// to surface per-task logs; the list and basic-detail endpoints omit them.
type assertoorRunDetail struct {
	Tasks []struct {
		Title       string `json:"title"`
		Result      string `json:"result"`
		ResultError string `json:"result_error"`
		Log         []struct {
			Level   string `json:"level"`
			Message string `json:"msg"`
		} `json:"log"`
	} `json:"tasks"`
}

// terminal reports whether an Assertoor test status is final (no longer changing).
func (r assertoorRun) terminal() bool {
	switch r.Status {
	case "success", "failure", "skipped", "aborted":
		return true
	default: // pending, running
		return false
	}
}

func (r assertoorRun) label() string {
	if r.Name != "" {
		return r.Name
	}
	return r.TestID
}

func (r assertoorRun) String() string {
	return fmt.Sprintf("%d=%s", r.RunID, r.Status)
}

// failedTasks returns "title (error)" for each task in the run that failed.
func (r assertoorRun) failedTasks() []string {
	var failed []string
	for _, task := range r.Tasks {
		if task.Result != "failure" {
			continue
		}
		if task.ResultError != "" {
			failed = append(failed, fmt.Sprintf("%s (%s)", task.Title, task.ResultError))
		} else {
			failed = append(failed, task.Title)
		}
	}
	return failed
}

// AssertoorEvent represents a scheduled Assertoor test run at a specific epoch.
type AssertoorEvent struct {
	Epoch    uint64
	Playbook string
	Config   map[string]any
}

// SortedAssertoorEvents returns a copy of events ordered by epoch, then playbook name.
func SortedAssertoorEvents(events []AssertoorEvent) []AssertoorEvent {
	sorted := slices.Clone(events)
	slices.SortFunc(sorted, func(a, b AssertoorEvent) int {
		if a.Epoch != b.Epoch {
			return cmp.Compare(a.Epoch, b.Epoch)
		}
		return cmp.Compare(a.Playbook, b.Playbook)
	})
	return sorted
}
