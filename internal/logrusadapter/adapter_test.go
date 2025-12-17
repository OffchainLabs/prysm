package logrusadapter_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/internal/logrusadapter"
	"github.com/sirupsen/logrus"
)

func TestLogrusAdapter(t *testing.T) {
	var outBuf bytes.Buffer
	l := logrus.Logger{
		Out:       &outBuf,
		Formatter: &logrus.TextFormatter{},
		Level:     logrus.DebugLevel,
	}

	slogger := slog.New(logrusadapter.Handler{Logger: &l})
	slogger.Error("test")

	if !strings.Contains(outBuf.String(), "test") {
		t.Errorf("unexpected output: %s", outBuf.String())
	}
}

func TestLevelMapping(t *testing.T) {
	tests := []struct {
		name        string
		slogLevel   slog.Level
		logrusLevel logrus.Level
		message     string
		wantInLog   string
	}{
		{
			name:        "Debug level",
			slogLevel:   slog.LevelDebug,
			logrusLevel: logrus.DebugLevel,
			message:     "debug message",
			wantInLog:   "level=debug",
		},
		{
			name:        "Info level",
			slogLevel:   slog.LevelInfo,
			logrusLevel: logrus.InfoLevel,
			message:     "info message",
			wantInLog:   "level=info",
		},
		{
			name:        "Warn level",
			slogLevel:   slog.LevelWarn,
			logrusLevel: logrus.WarnLevel,
			message:     "warn message",
			wantInLog:   "level=warning",
		},
		{
			name:        "Error level",
			slogLevel:   slog.LevelError,
			logrusLevel: logrus.ErrorLevel,
			message:     "error message",
			wantInLog:   "level=error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outBuf bytes.Buffer
			l := logrus.Logger{
				Out:       &outBuf,
				Formatter: &logrus.TextFormatter{},
				Level:     tt.logrusLevel,
			}

			slogger := slog.New(logrusadapter.Handler{Logger: &l})

			// Log at the specified level
			switch tt.slogLevel {
			case slog.LevelDebug:
				slogger.Debug(tt.message)
			case slog.LevelInfo:
				slogger.Info(tt.message)
			case slog.LevelWarn:
				slogger.Warn(tt.message)
			case slog.LevelError:
				slogger.Error(tt.message)
			}

			output := outBuf.String()
			if !strings.Contains(output, tt.message) {
				t.Errorf("expected message %q not found in output: %s", tt.message, output)
			}
			if !strings.Contains(output, tt.wantInLog) {
				t.Errorf("expected level indicator %q not found in output: %s", tt.wantInLog, output)
			}
		})
	}
}

func TestEnabledLevels(t *testing.T) {
	tests := []struct {
		name            string
		logrusLevel     logrus.Level
		slogLevel       slog.Level
		shouldBeEnabled bool
	}{
		// When logrus is at DebugLevel, all levels should be enabled
		{name: "Debug logger, debug level", logrusLevel: logrus.DebugLevel, slogLevel: slog.LevelDebug, shouldBeEnabled: true},
		{name: "Debug logger, info level", logrusLevel: logrus.DebugLevel, slogLevel: slog.LevelInfo, shouldBeEnabled: true},
		{name: "Debug logger, warn level", logrusLevel: logrus.DebugLevel, slogLevel: slog.LevelWarn, shouldBeEnabled: true},
		{name: "Debug logger, error level", logrusLevel: logrus.DebugLevel, slogLevel: slog.LevelError, shouldBeEnabled: true},

		// When logrus is at InfoLevel, debug should be disabled
		{name: "Info logger, debug level", logrusLevel: logrus.InfoLevel, slogLevel: slog.LevelDebug, shouldBeEnabled: false},
		{name: "Info logger, info level", logrusLevel: logrus.InfoLevel, slogLevel: slog.LevelInfo, shouldBeEnabled: true},
		{name: "Info logger, warn level", logrusLevel: logrus.InfoLevel, slogLevel: slog.LevelWarn, shouldBeEnabled: true},
		{name: "Info logger, error level", logrusLevel: logrus.InfoLevel, slogLevel: slog.LevelError, shouldBeEnabled: true},

		// When logrus is at WarnLevel, debug and info should be disabled
		{name: "Warn logger, debug level", logrusLevel: logrus.WarnLevel, slogLevel: slog.LevelDebug, shouldBeEnabled: false},
		{name: "Warn logger, info level", logrusLevel: logrus.WarnLevel, slogLevel: slog.LevelInfo, shouldBeEnabled: false},
		{name: "Warn logger, warn level", logrusLevel: logrus.WarnLevel, slogLevel: slog.LevelWarn, shouldBeEnabled: true},
		{name: "Warn logger, error level", logrusLevel: logrus.WarnLevel, slogLevel: slog.LevelError, shouldBeEnabled: true},

		// When logrus is at ErrorLevel, only error should be enabled
		{name: "Error logger, debug level", logrusLevel: logrus.ErrorLevel, slogLevel: slog.LevelDebug, shouldBeEnabled: false},
		{name: "Error logger, info level", logrusLevel: logrus.ErrorLevel, slogLevel: slog.LevelInfo, shouldBeEnabled: false},
		{name: "Error logger, warn level", logrusLevel: logrus.ErrorLevel, slogLevel: slog.LevelWarn, shouldBeEnabled: false},
		{name: "Error logger, error level", logrusLevel: logrus.ErrorLevel, slogLevel: slog.LevelError, shouldBeEnabled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outBuf bytes.Buffer
			l := logrus.Logger{
				Out:       &outBuf,
				Formatter: &logrus.TextFormatter{},
				Level:     tt.logrusLevel,
			}

			handler := logrusadapter.Handler{Logger: &l}
			enabled := handler.Enabled(context.Background(), tt.slogLevel)

			if enabled != tt.shouldBeEnabled {
				t.Errorf("Enabled() = %v, want %v for logrus level %v and slog level %v",
					enabled, tt.shouldBeEnabled, tt.logrusLevel, tt.slogLevel)
			}

			// Verify that disabled logs don't actually produce output
			slogger := slog.New(handler)
			switch tt.slogLevel {
			case slog.LevelDebug:
				slogger.Debug("test message")
			case slog.LevelInfo:
				slogger.Info("test message")
			case slog.LevelWarn:
				slogger.Warn("test message")
			case slog.LevelError:
				slogger.Error("test message")
			}

			hasOutput := strings.Contains(outBuf.String(), "test message")
			if hasOutput != tt.shouldBeEnabled {
				t.Errorf("Log output presence = %v, want %v", hasOutput, tt.shouldBeEnabled)
			}
		})
	}
}
