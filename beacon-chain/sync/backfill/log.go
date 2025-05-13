package backfill

import (
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "backfill")

// intervalLogger only logs once for each interval. It only customizes a single
// instance of the entry/logger and should just be used to control the logging rate for
// *one specific line of code*.
type intervalLogger struct {
	*logrus.Entry
	seconds int64         // seconds is the number of seconds per logging interval
	last    *atomic.Int64 // last is the quantized representation of the last time a log was emitted
}

func newIntervalLogger(base *logrus.Entry, secondsBetweenLogs int64) *intervalLogger {
	return &intervalLogger{
		Entry:   base,
		seconds: secondsBetweenLogs,
		last:    new(atomic.Int64),
	}
}

// Log overloads the Log() method of logrus.Entry, which is called under the hood
// when a log-level specific method (like Info(), Warn(), Error()) is invoked.
// By intercepting this call we can rate limit how often we log.
func (l *intervalLogger) Log(level logrus.Level, args ...interface{}) {
	// last is computed as the integer division of the current unix timestamp
	// divided by the number of seconds per interval.
	current := time.Now().Unix() / l.seconds
	// If Swap yields a different value, then we haven't yet logged within
	// the current window. Swap atomically sets the value so we can just
	// delegate the call and we're done.
	if l.last.Swap(current) != current {
		l.Logger.Log(level, args...)
	}
}
