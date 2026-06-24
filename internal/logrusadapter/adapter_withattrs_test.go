package logrusadapter_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/internal/logrusadapter"
	"github.com/sirupsen/logrus"
)

// TestWithAttrsPreservesFields verifies that fields added via slog.Logger.With
// appear in subsequent log output. This catches the bug where WithAttrs
// returns Handler{Logger: entry.Logger}, discarding the entry's fields.
func TestWithAttrsPreservesFields(t *testing.T) {
	var buf bytes.Buffer
	l := &logrus.Logger{
		Out:       &buf,
		Formatter: &logrus.TextFormatter{DisableTimestamp: true},
		Level:     logrus.DebugLevel,
	}

	base := slog.New(logrusadapter.Handler{Logger: l})
	child := base.With("component", "test-component")
	child.Info("hello")

	output := buf.String()
	if !strings.Contains(output, "component") || !strings.Contains(output, "test-component") {
		t.Errorf("WithAttrs field lost in output.\ngot: %s\nwant output to contain: component=test-component", output)
	}
}

// TestWithAttrsChained verifies that chaining multiple With calls accumulates fields.
func TestWithAttrsChained(t *testing.T) {
	var buf bytes.Buffer
	l := &logrus.Logger{
		Out:       &buf,
		Formatter: &logrus.TextFormatter{DisableTimestamp: true},
		Level:     logrus.DebugLevel,
	}

	logger := slog.New(logrusadapter.Handler{Logger: l})
	logger = logger.With("a", "1")
	logger = logger.With("b", "2")
	logger.Info("chained")

	output := buf.String()
	for _, want := range []string{"a", "1", "b", "2"} {
		if !strings.Contains(output, want) {
			t.Errorf("chained WithAttrs missing %q in output: %s", want, output)
		}
	}
}
