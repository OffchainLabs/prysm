package p2p

import (
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

const dataColumnBroadcastLogMessage = "Broadcasted data column sidecars summary"

func setDebugLogLevel(t *testing.T) {
	level := logrus.GetLevel()
	logrus.SetLevel(logrus.DebugLevel)
	t.Cleanup(func() { logrus.SetLevel(level) })
}

// waitForLogEntries waits until the hook contains at least count entries, or the deadline is reached.
func waitForLogEntries(hook *logTest.Hook, count int) []*logrus.Entry {
	deadline := time.Now().Add(5 * time.Second)
	for len(hook.AllEntries()) < count && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	return hook.AllEntries()
}

func TestDataColumnBroadcastLoggerRecord(t *testing.T) {
	setDebugLogLevel(t)

	t.Run("aggregates per root", func(t *testing.T) {
		hook := logTest.NewGlobal()

		rootA := [fieldparams.RootLength]byte{0x01}
		rootB := [fieldparams.RootLength]byte{0x02}

		// Long quiet period so that only explicit flushes log.
		l := &dataColumnBroadcastLogger{quietPeriod: time.Hour}
		l.record(rootA, 10, 5, 300*time.Millisecond)
		l.record(rootA, 10, 1, 100*time.Millisecond)
		l.record(rootB, 11, 7, 200*time.Millisecond)
		l.record(rootA, 10, 3, 200*time.Millisecond)

		require.Equal(t, 0, len(hook.AllEntries()))

		l.flush(rootA, l.pending[rootA])
		entries := hook.AllEntries()
		require.Equal(t, 1, len(entries))
		require.Equal(t, dataColumnBroadcastLogMessage, entries[0].Message)
		require.Equal(t, "0x01000000...", entries[0].Data["root"])
		require.Equal(t, 3, entries[0].Data["count"])
		require.Equal(t, "1,3,5", entries[0].Data["indices"])
		require.Equal(t, "[min: 100ms, avg: 200ms, max: 300ms]", entries[0].Data["sinceStartTime"])

		l.flush(rootB, l.pending[rootB])
		entries = hook.AllEntries()
		require.Equal(t, 2, len(entries))
		require.Equal(t, 1, entries[1].Data["count"])
		require.Equal(t, "7", entries[1].Data["indices"])
		require.Equal(t, "[min: 200ms, avg: 200ms, max: 200ms]", entries[1].Data["sinceStartTime"])
	})

	t.Run("logs once the quiet period has elapsed", func(t *testing.T) {
		hook := logTest.NewGlobal()

		root := [fieldparams.RootLength]byte{0x03}

		l := &dataColumnBroadcastLogger{quietPeriod: 10 * time.Millisecond}
		l.record(root, 12, 4, 100*time.Millisecond)
		l.record(root, 12, 2, 150*time.Millisecond)

		entries := waitForLogEntries(hook, 1)
		require.Equal(t, 1, len(entries))
		require.Equal(t, 2, entries[0].Data["count"])
		require.Equal(t, "2,4", entries[0].Data["indices"])
		require.Equal(t, "[min: 100ms, avg: 125ms, max: 150ms]", entries[0].Data["sinceStartTime"])

		l.mu.Lock()
		defer l.mu.Unlock()
		require.Equal(t, 0, len(l.pending))
	})

	t.Run("each broadcast restarts the quiet period", func(t *testing.T) {
		hook := logTest.NewGlobal()

		root := [fieldparams.RootLength]byte{0x04}

		const quietPeriod = 500 * time.Millisecond
		l := &dataColumnBroadcastLogger{quietPeriod: quietPeriod}

		l.record(root, 13, 1, 100*time.Millisecond)
		time.Sleep(quietPeriod * 3 / 5)
		l.record(root, 13, 3, 200*time.Millisecond)
		time.Sleep(quietPeriod * 3 / 5)

		// More than the quiet period elapsed since the first broadcast, but not since the second one.
		require.Equal(t, 0, len(hook.AllEntries()))

		entries := waitForLogEntries(hook, 1)
		require.Equal(t, 1, len(entries))
		require.Equal(t, 2, entries[0].Data["count"])
		require.Equal(t, "1,3", entries[0].Data["indices"])
	})
}

func TestDataColumnBroadcastLoggerFlush(t *testing.T) {
	setDebugLogLevel(t)

	t.Run("unknown root", func(t *testing.T) {
		hook := logTest.NewGlobal()

		l := &dataColumnBroadcastLogger{}
		l.flush([fieldparams.RootLength]byte{0x05}, &dataColumnBroadcastLogInfo{})
		require.Equal(t, 0, len(hook.AllEntries()))
	})

	t.Run("stale entry", func(t *testing.T) {
		hook := logTest.NewGlobal()

		root := [fieldparams.RootLength]byte{0x06}

		l := &dataColumnBroadcastLogger{quietPeriod: time.Hour}
		l.record(root, 14, 1, 100*time.Millisecond)
		stale := l.pending[root]
		l.flush(root, stale)
		require.Equal(t, 1, len(hook.AllEntries()))

		// A new entry for the same root must not be flushed by a timer belonging to the previous entry.
		l.record(root, 14, 2, 200*time.Millisecond)
		l.flush(root, stale)
		require.Equal(t, 1, len(hook.AllEntries()))
		require.Equal(t, 1, len(l.pending))
	})
}
