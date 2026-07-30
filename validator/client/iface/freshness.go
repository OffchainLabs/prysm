package iface

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

type hintKey struct{}

// Head is the head a read is expected to report.
type Head struct {
	Root          [32]byte
	Slot          primitives.Slot
	PayloadStatus api.PayloadStatus
}

// Hint is the request-scoped freshness expectation.
type Hint struct {
	Head     func() (Head, bool)
	Deadline time.Time
}

// WithHint annotates ctx with a freshness expectation.
func WithHint(ctx context.Context, h Hint) context.Context {
	if h.Head == nil {
		return ctx
	}

	return context.WithValue(ctx, hintKey{}, h)
}

// FromContext returns the freshness hint set on ctx, if any.
func FromContext(ctx context.Context) (Hint, bool) {
	hint, ok := ctx.Value(hintKey{}).(Hint)
	return hint, ok
}
