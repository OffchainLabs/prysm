package archive

import (
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// rateLimitedLogger emits at most one line per interval. It is meant for a single call site, so that a long
// running walk reports progress without flooding the log.
type rateLimitedLogger struct {
	base     *logrus.Entry
	entry    *logrus.Entry
	interval int64
	last     *atomic.Int64
}

func newRateLimitedLogger(base *logrus.Entry, interval time.Duration) *rateLimitedLogger {
	return &rateLimitedLogger{
		base:     base,
		entry:    base,
		interval: int64(interval.Seconds()),
		last:     new(atomic.Int64),
	}
}

func (l *rateLimitedLogger) WithFields(fields logrus.Fields) *rateLimitedLogger {
	return &rateLimitedLogger{
		base:     l.base,
		entry:    l.entry.WithFields(fields),
		interval: l.interval,
		last:     l.last,
	}
}

func (l *rateLimitedLogger) Info(args ...any) {
	n := time.Now().Unix() / l.interval
	if l.last.Swap(n) != n {
		l.entry.Info(args...)
	}
}
