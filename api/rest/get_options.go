package rest

import (
	"encoding/json"
	"net/http"
	"time"
)

const defaultPollInterval = 50 * time.Millisecond // backoff between re-polling

// RepollMode selects the condition under which a re-polling read keeps retrying.
type RepollMode int

const (
	UntilAccepted RepollMode = iota // Re-polls until an accept-passing response is received or the deadline fires.
	UntilAny2xx   RepollMode = iota // Re-polls only while no usable (2xx) response has been received at all.
)

// getConfig holds the per-call configuration.
type getConfig struct {
	race             bool                                    // When true, query all nodes concurrently. When false, try nodes in order.
	accept           func(json.RawMessage) bool              // Acceptance criterion for a JSON Get.
	sszAccept        func(body []byte, hdr http.Header) bool // Acceptance criterion for a GetSSZ.
	pollInterval     time.Duration                           // When > 0, keep re-polling all nodes until the deadline, waiting this long between rounds.
	repollMode       RepollMode                              // When re-polling, the condition under which retrying stops.
	deadline         time.Time                               // Absolute instant by which the read must finish
	fallbackDeadline time.Time                               // If non-zero, bounds the wait for the nodes that have not answered yet once a usable response is in hand.
}

// GetOption customizes a Handler.Get call.
type GetOption func(*getConfig)

// newGetConfig folds opts into a getConfig.
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

// WithFallbackDeadline bounds how long a read waits for the nodes that have not
// answered yet, once another node has already returned a usable (but not
// accepted) response.
func WithFallbackDeadline(t time.Time) GetOption {
	return func(c *getConfig) {
		c.fallbackDeadline = t
	}
}

// WithRepoll makes a read keep re-polling all nodes (at defaultPollInterval)
// until the deadline fires. mode selects what stops the re-polling: an
// accept-passing response (UntilAccepted) or any usable 2xx response
// (UntilAny2xx). WithRepoll has no effect unless WithDeadline is also set.
func WithRepoll(mode RepollMode) GetOption {
	return func(c *getConfig) {
		c.pollInterval = defaultPollInterval
		c.repollMode = mode
	}
}

// ResolvedConfig is a read-only view of the configuration a set of GetOptions
// produces. It lets other packages unit-test the options they build without
// exercising a full HTTP read (getConfig itself is unexported).
type ResolvedConfig struct {
	Race             bool
	Accept           func(raw json.RawMessage) bool
	SSZAccept        func(body []byte, hdr http.Header) bool
	PollInterval     time.Duration
	RepollMode       RepollMode
	Deadline         time.Time
	FallbackDeadline time.Time
}

// ResolveOptions folds opts into a ResolvedConfig for inspection.
func ResolveOptions(opts ...GetOption) ResolvedConfig {
	cfg := newGetConfig(opts)

	return ResolvedConfig{
		Race:             cfg.race,
		Accept:           cfg.accept,
		SSZAccept:        cfg.sszAccept,
		PollInterval:     cfg.pollInterval,
		RepollMode:       cfg.repollMode,
		Deadline:         cfg.deadline,
		FallbackDeadline: cfg.fallbackDeadline,
	}
}
