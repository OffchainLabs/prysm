package logging

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// IntervalLogger only logs once for each interval. It only customizes a single
// instance of the entry/logger and should just be used to control the logging rate for
// *one specific line of code*.
type IntervalLogger struct {
	*logrus.Entry
	base    *logrus.Entry
	mux     sync.Mutex
	seconds int64         // seconds is the number of seconds per logging interval
	last    *atomic.Int64 // last is the quantized representation of the last time a log was emitted
	now     func() time.Time
}

func NewIntervalLogger(base *logrus.Entry, secondsBetweenLogs int64) *IntervalLogger {
	return &IntervalLogger{
		Entry:   base,
		base:    base,
		seconds: secondsBetweenLogs,
		last:    new(atomic.Int64),
		now:     time.Now,
	}
}

// intervalNumber is a separate pure function because this helps tests determine
// proposer timestamp alignment.
func intervalNumber(t time.Time, seconds int64) int64 {
	return t.Unix() / seconds
}

// intervalNumber is the integer division of the current unix timestamp
// divided by the number of seconds per interval.
func (l *IntervalLogger) intervalNumber() int64 {
	return intervalNumber(l.now(), l.seconds)
}

func (l *IntervalLogger) copy() *IntervalLogger {
	return &IntervalLogger{
		Entry:   l.Entry,
		base:    l.base,
		seconds: l.seconds,
		last:    l.last,
		now:     l.now,
	}
}

// Log overloads the Log() method of logrus.Entry, which is called under the hood
// when a log-level specific method (like Info(), Warn(), Error()) is invoked.
// By intercepting this call we can rate limit how often we log.
func (l *IntervalLogger) Log(level logrus.Level, args ...any) {
	n := l.intervalNumber()
	// If Swap returns a different value that the current interval number, we haven't
	// emitted a log yet this interval, so we can do so now.
	if l.last.Swap(n) != n {
		l.Entry.Log(level, args...)
	}
	// reset the Entry to the base so that any WithField/WithError calls
	// don't persist across calls to Log()
}

func (l *IntervalLogger) WithField(key string, value any) *IntervalLogger {
	cp := l.copy()
	cp.Entry = cp.Entry.WithField(key, value)
	return cp
}

func (l *IntervalLogger) WithFields(fields logrus.Fields) *IntervalLogger {
	cp := l.copy()
	cp.Entry = cp.Entry.WithFields(fields)
	return cp
}

func (l *IntervalLogger) WithError(err error) *IntervalLogger {
	cp := l.copy()
	cp.Entry = cp.Entry.WithError(err)
	return cp
}

func (l *IntervalLogger) Trace(args ...any) {
	l.Log(logrus.TraceLevel, args...)
}

func (l *IntervalLogger) Debug(args ...any) {
	l.Log(logrus.DebugLevel, args...)
}

func (l *IntervalLogger) Print(args ...any) {
	l.Info(args...)
}

func (l *IntervalLogger) Info(args ...any) {
	l.Log(logrus.InfoLevel, args...)
}

func (l *IntervalLogger) Warn(args ...any) {
	l.Log(logrus.WarnLevel, args...)
}

func (l *IntervalLogger) Warning(args ...any) {
	l.Warn(args...)
}

func (l *IntervalLogger) Error(args ...any) {
	l.Log(logrus.ErrorLevel, args...)
}
