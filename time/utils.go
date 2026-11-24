// Package time is a wrapper around the go standard time library.
package time

import (
	"fmt"
	"time"
)

// Since returns the duration since t.
func Since(t time.Time) time.Duration {
	return Now().Sub(t)
}

// Until returns the duration until t.
func Until(t time.Time) time.Duration {
	return t.Sub(Now())
}

// Now returns the current local time.
func Now() time.Time {
	return time.Now()
}

// PrettyMinMaxAverage returns `min`, `average` and `max` as one string, to log them in a single field.
func PrettyMinMaxAverage(min, max, average time.Duration) string {
	return fmt.Sprintf("[min: %v, avg: %v, max: %v]", min, average, max)
}
