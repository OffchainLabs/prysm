package rest

import (
	"encoding/json"
	"net/http"
	"time"
)

const defaultPollInterval = 50 * time.Millisecond // backoff between re-polling

// getConfig holds the per-call configuration.
type getConfig struct {
	race         bool                                    // When true, query all nodes concurrently. When false, try nodes in order.
	accept       func(json.RawMessage) bool              // Acceptance criterion for a JSON Get. Never nil: defaults to accept-all, narrowed by WithAccept.
	sszAccept    func(body []byte, hdr http.Header) bool // Acceptance criterion for a GetSSZ. Never nil: defaults to accept-all, narrowed by WithSSZAccept.
	pollInterval time.Duration                           // When > 0, keep re-polling all nodes until the deadline, waiting this long between rounds.
	deadline     time.Time                               // Absolute instant by which the read must finish
}

// GetOption customizes a Handler.Get call.
type GetOption func(*getConfig)

// newGetConfig folds opts into a getConfig. The acceptance criteria default to
// accept-all (any 2XX); WithAccept/WithSSZAccept overwrite them.
func newGetConfig(opts []GetOption) getConfig {
	// By default, accept any 2XX response.
	cfg := getConfig{
		accept:    func(json.RawMessage) bool { return true },
		sszAccept: func([]byte, http.Header) bool { return true },
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

// WithRace makes the read query all beacon nodes concurrently.
// Without it, it reads try nodes in order.
func WithRace() GetOption {
	return func(c *getConfig) {
		c.race = true
	}
}

// WithAccept narrows Get's acceptance criterion from any 2XX response to a 2XX
// response whose body also satisfies accept.
func WithAccept(accept func(raw json.RawMessage) bool) GetOption {
	return func(c *getConfig) {
		if accept != nil {
			c.accept = accept
		}
	}
}

// WithSSZAccept narrows GetSSZ's acceptance criterion from any 2XX response to
// a 2XX response whose body also satisfies accept.
func WithSSZAccept(accept func(body []byte, header http.Header) bool) GetOption {
	return func(c *getConfig) {
		if accept != nil {
			c.sszAccept = accept
		}
	}
}

// WithDeadline sets a deadline for the read.
func WithDeadline(t time.Time) GetOption {
	return func(c *getConfig) {
		c.deadline = t
	}
}

// WithRepoll makes a read keep re-polling all nodes until one is accepted or the
// deadline fires.
func WithRepoll(interval time.Duration) GetOption {
	return func(c *getConfig) {
		if interval <= 0 {
			interval = defaultPollInterval
		}

		c.pollInterval = interval
	}
}
