package kurtosis

import (
	"encoding/json"
	"fmt"
)

// assertoorEnvelope is the {status, data} wrapper Assertoor puts around every API response.
type assertoorEnvelope struct {
	Data json.RawMessage `json:"data"`
}

// assertoorRun is the subset of an Assertoor test run we care about.
type assertoorRun struct {
	RunID  uint64          `json:"run_id"`
	TestID string          `json:"test_id"`
	Name   string          `json:"name"`
	Status string          `json:"status"`
	Tasks  []assertoorTask `json:"tasks,omitempty"`
}

type assertoorTask struct {
	Title       string `json:"title"`
	Result      string `json:"result"`
	ResultError string `json:"result_error"`
	Log         []struct {
		Level   string `json:"level"`
		Message string `json:"msg"`
	} `json:"log"`
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

func (r assertoorRun) failed() bool {
	return r.Status == "failure" || r.Status == "aborted"
}

func (r assertoorRun) label() string {
	if r.Name != "" {
		return r.Name
	}
	return r.TestID
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
