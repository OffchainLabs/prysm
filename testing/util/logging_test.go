package util

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/sirupsen/logrus"
)

func TestUnregister(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel) // set to lowest log level to test level override in
	assertNoHooks(t, logger)
	c := make(chan *logrus.Entry, 1)
	tl := NewChannelEntryWriter(c)
	undo := RegisterHookWithUndo(logger, tl)
	assertRegistered(t, logger, tl)
	logger.Trace("test")
	select {
	case <-c:
	default:
		t.Fatalf("Expected log entry, got none")
	}
	undo()
	assertNoHooks(t, logger)
	require.Equal(t, logrus.PanicLevel, logger.Level)
}

var logTestErr = errors.New("test")

func TestChannelEntryWriter(t *testing.T) {
	logger := logrus.New()
	c := make(chan *logrus.Entry)
	tl := NewChannelEntryWriter(c)
	logger.AddHook(tl)
	msg := "test"
	go func() {
		logger.WithError(logTestErr).Info(msg)
	}()
	select {
	case e := <-c:
		gotErr := e.Data[logrus.ErrorKey]
		if gotErr == nil {
			t.Fatalf("Expected error in log entry, got nil")
		}
		ge, ok := gotErr.(error)
		require.Equal(t, true, ok, "Expected error in log entry to be of type error, got %T", gotErr)
		require.ErrorIs(t, ge, logTestErr)
		require.Equal(t, msg, e.Message)
		require.Equal(t, logrus.InfoLevel, e.Level)
	case <-time.After(10 * time.Millisecond):
		t.Fatalf("Timed out waiting for log entry")
	}
}

func assertNoHooks(t *testing.T, logger *logrus.Logger) {
	for lvl, hooks := range logger.Hooks {
		for _, hook := range hooks {
			t.Fatalf("Expected no hooks, got %v at level %s", hook, lvl.String())
		}
	}
}

func assertRegistered(t *testing.T, logger *logrus.Logger, hook ComparableHook) {
	for _, lvl := range hook.Levels() {
		registered := logger.Hooks[lvl]
		found := false
		for _, h := range registered {
			if hook.Equal(h) {
				found = true
				break
			}
		}
		require.Equal(t, true, found, "Expected hook %v to be registered at level %s, but it was not", hook, lvl.String())
	}
}
