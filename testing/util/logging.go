package util

import (
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

// ComparableHook is an interface that allows hooks to be uniquely identified
// so that tests can safely unregister them as part of cleanup.
type ComparableHook interface {
	logrus.Hook
	Equal(other logrus.Hook) bool
}

// UnregisterHook removes a hook that implements the HookIdentifier interface
// from all levels of the given logger.
func UnregisterHook(logger *logrus.Logger, unregister ComparableHook) {
	found := false
	replace := make(logrus.LevelHooks)
	for lvl, hooks := range logger.Hooks {
		for _, h := range hooks {
			if unregister.Equal(h) {
				found = true
				continue
			}
			replace[lvl] = append(replace[lvl], h)
		}
	}
	if !found {
		return
	}
	logger.ReplaceHooks(replace)
}

var highestLevel logrus.Level

// RegisterHookWithUndo adds a hook to the logger and
// returns a function that can be called to remove it. This is intended to be used in tests
// to ensure that test hooks are removed after the test is complete.
func RegisterHookWithUndo(logger *logrus.Logger, hook ComparableHook) func() {
	level := logger.Level
	logger.AddHook(hook)
	// set level to highest possible to ensure that hook is called for all log levels
	logger.SetLevel(highestLevel)
	return func() {
		UnregisterHook(logger, hook)
		logger.SetLevel(level)
	}
}

// NewChannelEntryWriter creates a new ChannelEntryWriter.
// The channel argument will be sent all log entries.
// Note that if this is an unbuffered channel, it is the responsibility
// of the code using it to make sure that it is drained appropriately,
// or calls to the logger can block.
func NewChannelEntryWriter(c chan *logrus.Entry) *ChannelEntryWriter {
	return &ChannelEntryWriter{c: c}
}

// ChannelEntryWriter embeds/wraps the test.Hook struct
// and adds a channel to receive log entries every time the
// Fire method of the Hook interface is called.
type ChannelEntryWriter struct {
	test.Hook
	c chan *logrus.Entry
}

// Fire delegates to the embedded test.Hook Fire method after
// sending the log entry to the channel.
func (c *ChannelEntryWriter) Fire(e *logrus.Entry) error {
	if c.c != nil {
		c.c <- e
	}
	return c.Hook.Fire(e)
}

func (c *ChannelEntryWriter) Equal(other logrus.Hook) bool {
	return c == other
}

var _ logrus.Hook = &ChannelEntryWriter{}
var _ ComparableHook = &ChannelEntryWriter{}

func init() {
	for _, level := range logrus.AllLevels {
		if level > highestLevel {
			highestLevel = level
		}
	}
}
