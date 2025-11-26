package backfill

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// trackingHook is a logrus hook that counts Log callCount for testing.
type trackingHook struct {
	mu      sync.RWMutex
	entries []*logrus.Entry
}

func (h *trackingHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *trackingHook) Fire(entry *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, entry)
	return nil
}

func (h *trackingHook) callCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.entries)
}

func (h *trackingHook) emitted(t *testing.T) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	e := make([]string, len(h.entries))
	for i, entry := range h.entries {
		entry.Buffer = new(bytes.Buffer)
		serialized, err := entry.Logger.Formatter.Format(entry)
		require.NoError(t, err)
		e[i] = string(serialized)
	}
	return e
}

func entryWithHook() (*logrus.Entry, *trackingHook) {
	logger := logrus.New()
	logger.SetLevel(logrus.TraceLevel)
	hook := &trackingHook{}
	logger.AddHook(hook)
	entry := logrus.NewEntry(logger)
	return entry, hook
}

func intervalSecondsAndDuration(i int) (int64, time.Duration) {
	return int64(i), time.Duration(i) * time.Second
}

// mockClock provides a controllable time source for testing.
// It allows tests to set the current time and advance it as needed.
type mockClock struct {
	t time.Time
}

// now returns the current time.
func (c *mockClock) now() time.Time {
	return c.t
}

func setupMockClock(il *intervalLogger) *mockClock {
	// initialize now so that the time aligns with the start of the
	// interval bucket. This ensures that adding less than an interval
	// of time to the timestamp can never move into the next bucket.
	interval := intervalNumber(time.Now(), il.seconds)
	now := time.Unix(interval*il.seconds, 0)
	clock := &mockClock{t: now}
	il.now = clock.now
	return clock
}

// TestNewIntervalLogger verifies logger is properly initialized
func TestNewIntervalLogger(t *testing.T) {
	base := logrus.NewEntry(logrus.New())
	intSec := int64(10)

	il := newIntervalLogger(base, intSec)

	require.NotNil(t, il)
	require.Equal(t, intSec, il.seconds)
	require.Equal(t, int64(0), il.last.Load())
	require.Equal(t, base, il.Entry)
}

// TestLogOncePerInterval verifies that Log is called only once within an interval window
func TestLogOncePerInterval(t *testing.T) {
	entry, hook := entryWithHook()

	il := newIntervalLogger(entry, 10)
	_ = setupMockClock(il) // use a fixed time to make sure no race is possible

	// First log should call the underlying Log method
	il.Log(logrus.InfoLevel, "test message 1")
	require.Equal(t, 1, hook.callCount())

	// Second log in same interval should not call Log
	il.Log(logrus.InfoLevel, "test message 2")
	require.Equal(t, 1, hook.callCount())

	// Third log still in same interval should not call Log
	il.Log(logrus.InfoLevel, "test message 3")
	require.Equal(t, 1, hook.callCount())

	// Verify last is set to current interval
	require.Equal(t, il.intervalNumber(), il.last.Load())
}

// TestLogAcrossIntervalBoundary verifies logging at interval boundaries resets correctly
func TestLogAcrossIntervalBoundary(t *testing.T) {
	iSec, iDur := intervalSecondsAndDuration(10)

	entry, hook := entryWithHook()
	il := newIntervalLogger(entry, iSec)
	clock := setupMockClock(il)

	il.Log(logrus.InfoLevel, "first interval")
	require.Equal(t, 1, hook.callCount())

	// Log in new interval should succeed
	clock.t = clock.t.Add(2 * iDur)
	il.Log(logrus.InfoLevel, "second interval")
	require.Equal(t, 2, hook.callCount())
}

// TestWithFieldChaining verifies WithField returns logger and can be chained
func TestWithFieldChaining(t *testing.T) {
	entry, hook := entryWithHook()
	iSec, iDur := intervalSecondsAndDuration(10)
	il := newIntervalLogger(entry, iSec)
	clock := setupMockClock(il)

	result := il.WithField("key1", "value1")
	require.NotNil(t, result)
	result.Info("test")
	require.Equal(t, 1, hook.callCount())

	// make sure there was no mutation of the base as a side effect
	clock.t = clock.t.Add(iDur)
	il.Info("another")

	// Verify field is present in logged entry
	emitted := hook.emitted(t)
	require.Contains(t, emitted[0], "test")
	require.Contains(t, emitted[0], "key1=value1")
	require.Contains(t, emitted[1], "another")
	require.NotContains(t, emitted[1], "key1=value1")
}

// TestWithFieldsChaining verifies WithFields properly adds multiple fields
func TestWithFieldsChaining(t *testing.T) {
	entry, hook := entryWithHook()
	iSec, iDur := intervalSecondsAndDuration(10)
	il := newIntervalLogger(entry, iSec)
	clock := setupMockClock(il)

	fields := logrus.Fields{
		"key1": "value1",
		"key2": "value2",
	}
	result := il.WithFields(fields)
	require.NotNil(t, result)
	result.Info("test")
	require.Equal(t, 1, hook.callCount())

	// make sure there was no mutation of the base as a side effect
	clock.t = clock.t.Add(iDur)
	il.Info("another")

	// Verify field is present in logged entry
	emitted := hook.emitted(t)
	require.Contains(t, emitted[0], "test")
	require.Contains(t, emitted[0], "key1=value1")
	require.Contains(t, emitted[0], "key2=value2")
	require.Contains(t, emitted[1], "another")
	require.NotContains(t, emitted[1], "key1=value1")
	require.NotContains(t, emitted[1], "key2=value2")
}

// TestWithErrorChaining verifies WithError properly adds error field
func TestWithErrorChaining(t *testing.T) {
	entry, hook := entryWithHook()
	iSec, iDur := intervalSecondsAndDuration(10)
	il := newIntervalLogger(entry, iSec)
	clock := setupMockClock(il)

	expected := errors.New("lowercase words")
	result := il.WithError(expected)
	require.NotNil(t, result)
	result.Error("test")
	require.Equal(t, 1, hook.callCount())

	require.NotNil(t, result)

	// make sure there was no mutation of the base as a side effect
	clock.t = clock.t.Add(iDur)
	il.Info("different")

	// Verify field is present in logged entry
	emitted := hook.emitted(t)
	require.Contains(t, emitted[0], expected.Error())
	require.Contains(t, emitted[0], "test")
	require.Contains(t, emitted[1], "different")
	require.NotContains(t, emitted[1], "test")
	require.NotContains(t, emitted[1], "lowercase words")
}

// TestLogLevelMethods verifies all log level methods work and respect rate limiting
func TestLogLevelMethods(t *testing.T) {
	entry, hook := entryWithHook()
	il := newIntervalLogger(entry, 10)
	_ = setupMockClock(il) // use a fixed time to make sure no race is possible

	// First call from each level-specific method should succeed
	il.Trace("trace message")
	require.Equal(t, 1, hook.callCount())

	// Subsequent callCount in same interval should be suppressed
	il.Debug("debug message")
	require.Equal(t, 1, hook.callCount())

	il.Info("info message")
	require.Equal(t, 1, hook.callCount())

	il.Print("print message")
	require.Equal(t, 1, hook.callCount())

	il.Warn("warn message")
	require.Equal(t, 1, hook.callCount())

	il.Warning("warning message")
	require.Equal(t, 1, hook.callCount())

	il.Error("error message")
	require.Equal(t, 1, hook.callCount())
}

// TestConcurrentLogging verifies multiple goroutines can safely call Log concurrently
func TestConcurrentLogging(t *testing.T) {
	entry, hook := entryWithHook()
	il := newIntervalLogger(entry, 10)
	_ = setupMockClock(il) // use a fixed time to make sure no race is possible

	var wg sync.WaitGroup
	wait := make(chan struct{})
	for range 10 {
		wg.Add(1)
		go func() {
			<-wait
			defer wg.Done()
			il.Log(logrus.InfoLevel, "concurrent message")
		}()
	}
	close(wait) // maximize raciness by unblocking goroutines together
	wg.Wait()

	// Only one Log call should succeed across all goroutines in the same interval
	require.Equal(t, 1, hook.callCount())
}

// TestZeroInterval verifies behavior with small interval (logs every second)
func TestZeroInterval(t *testing.T) {
	entry, hook := entryWithHook()
	il := newIntervalLogger(entry, 1)
	clock := setupMockClock(il)

	il.Log(logrus.InfoLevel, "first")
	require.Equal(t, 1, hook.callCount())

	// Move to next second
	clock.t = clock.t.Add(time.Second)
	il.Log(logrus.InfoLevel, "second")
	require.Equal(t, 2, hook.callCount())
}

// TestCompleteLoggingFlow tests realistic scenario with repeated logging
func TestCompleteLoggingFlow(t *testing.T) {
	entry, hook := entryWithHook()
	iSec, iDur := intervalSecondsAndDuration(10)
	il := newIntervalLogger(entry, iSec)
	clock := setupMockClock(il)

	// Add field
	il = il.WithField("request_id", "12345")

	// Log multiple times in same interval - only first succeeds
	il.Info("message 1")
	require.Equal(t, 1, hook.callCount())

	il.Warn("message 2")
	require.Equal(t, 1, hook.callCount())

	// Move to next interval
	clock.t = clock.t.Add(iDur)

	// Should be able to log again in new interval
	il.Error("message 3")
	require.Equal(t, 2, hook.callCount())

	require.NotNil(t, il)
}

// TestAtomicSwapCorrectness verifies atomic swap works correctly
func TestAtomicSwapCorrectness(t *testing.T) {
	il := newIntervalLogger(logrus.NewEntry(logrus.New()), 10)
	_ = setupMockClock(il) // use a fixed time to make sure no race is possible

	// Swap operation should return different value on first call
	current := il.intervalNumber()
	old := il.last.Swap(current)
	require.Equal(t, int64(0), old) // initial value is 0
	require.Equal(t, current, il.last.Load())

	// Swap with same value should return the same value
	old = il.last.Swap(current)
	require.Equal(t, current, old)
}

// TestLogMethodsWithClockAdvancement verifies that log methods respect rate limiting
// within an interval but emit again after the interval passes.
func TestLogMethodsWithClockAdvancement(t *testing.T) {
	entry, hook := entryWithHook()

	iSec, iDur := intervalSecondsAndDuration(10)
	il := newIntervalLogger(entry, iSec)
	clock := setupMockClock(il)

	// First Error call should log
	il.Error("error 1")
	require.Equal(t, 1, hook.callCount())

	// Warn call in same interval should be suppressed
	il.Warn("warn 1")
	require.Equal(t, 1, hook.callCount())

	// Info call in same interval should be suppressed
	il.Info("info 1")
	require.Equal(t, 1, hook.callCount())

	// Debug call in same interval should be suppressed
	il.Debug("debug 1")
	require.Equal(t, 1, hook.callCount())

	// Move forward 5 seconds - still in same 10-second interval
	require.Equal(t, 5*time.Second, iDur/2)
	clock.t = clock.t.Add(iDur / 2)
	il.Error("error 2")
	require.Equal(t, 1, hook.callCount(), "should still be suppressed within same interval")
	firstInterval := il.intervalNumber()

	// Move forward to next interval (10 second interval boundary)
	clock.t = clock.t.Add(iDur / 2)
	nextInterval := il.intervalNumber()
	require.NotEqual(t, firstInterval, nextInterval, "should be in new interval now")

	il.Error("error 3")
	require.Equal(t, 2, hook.callCount(), "should emit in new interval")

	// Another call in the new interval should be suppressed
	il.Warn("warn 2")
	require.Equal(t, 2, hook.callCount())

	// Move forward to yet another interval
	clock.t = clock.t.Add(iDur)
	il.Info("info 2")
	require.Equal(t, 3, hook.callCount(), "should emit in third interval")
}
