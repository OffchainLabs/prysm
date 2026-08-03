package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
)

type (
	multiHandler struct {
		handlers []*handler
	}

	sszResult struct {
		body   []byte
		header http.Header
	}

	queryFunc[T any] func(context.Context, *handler) (T, error)

	// readRound queries every handler once and reports the outcome:
	//   - matched=true: val is the first response satisfying accept.
	//   - matched=false, ok=true: no response matched, val is a best-effort success.
	//   - matched=false, ok=false: every handler failed; errs holds the failures.
	readRound[T any] func(ctx context.Context, handlers []*handler, fallbackDeadline time.Time, accept func(T) bool, fn queryFunc[T]) (val T, matched, ok bool, errs []error)
)

func newMultiHandler(handlers []*handler) (*multiHandler, error) {
	if len(handlers) == 0 {
		return nil, errors.New("multiHandler requires at least one handler")
	}

	return &multiHandler{handlers: handlers}, nil
}

// Host returns every endpoint the handler queries, comma-separated
func (m *multiHandler) Host() string {
	// Safe when `multiHandler` is constructed with `newMultiHandler`.
	hosts := make([]string, 0, len(m.handlers))
	for _, handler := range m.handlers {
		hosts = append(hosts, handler.Host())
	}

	return strings.Join(hosts, ",")
}

// Get reads a GET from the nodes.
// When resp is nil the response body is discarded.
func (m *multiHandler) Get(ctx context.Context, endpoint string, resp any, opts ...GetOption) error {
	cfg := newGetConfig(opts)

	get := func(ctx context.Context, handler *handler) (json.RawMessage, error) {
		// We don't care about the response body.
		if resp == nil {
			if err := handler.Get(ctx, endpoint, nil); err != nil {
				return nil, fmt.Errorf("get: %w", err)
			}

			return nil, nil
		}

		// We do care about the response body.
		raw, err := handler.getRaw(ctx, endpoint)
		if err != nil {
			return nil, fmt.Errorf("get: %w", err)
		}

		return raw, nil
	}

	// Query nodes.
	queryFunc := roundFor[json.RawMessage](cfg)
	raw, _, err := readUntil(ctx, m.handlers, cfg, cfg.accept, queryFunc, get)
	if err != nil {
		return fmt.Errorf("read until: %w", err)
	}

	// Decode the response into resp.
	if err := decodeInto(raw, resp); err != nil {
		return fmt.Errorf("decode into: %w", err)
	}

	return nil
}

// GetSSZ reads a GET from the nodes, requesting SSZ but accepting JSON.
func (m *multiHandler) GetSSZ(ctx context.Context, endpoint string, opts ...GetOption) ([]byte, http.Header, error) {
	cfg := newGetConfig(opts)

	get := func(ctx context.Context, h *handler) (sszResult, error) {
		body, header, err := h.GetSSZ(ctx, endpoint)
		if err != nil {
			return sszResult{}, fmt.Errorf("get ssz: %w", err)
		}

		return sszResult{body: body, header: header}, nil
	}

	// Adapt the header/body predicate to the sszResult the round produces.
	accept := func(r sszResult) bool { return cfg.sszAccept(r.body, r.header) }

	res, _, err := readUntil(ctx, m.handlers, cfg, accept, roundFor[sszResult](cfg), get)
	if err != nil {
		return nil, nil, fmt.Errorf("read until: %w", err)
	}

	return res.body, res.header, nil
}

// GetStatusCode queries all nodes and returns 200 if any node is ready,
// otherwise the last non-200 status observed, or a joined error if every node
// failed at the transport level.
func (m *multiHandler) GetStatusCode(ctx context.Context, endpoint string) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		code int
		err  error
	}

	// Asynchronously query every handler and send the result to results.
	results := make(chan result, len(m.handlers))
	for _, h := range m.handlers {
		go func(h *handler) {
			code, err := h.GetStatusCode(ctx, endpoint)
			results <- result{code: code, err: err}
		}(h)
	}

	var (
		lastCode int
		errs     []error
	)

	// Pull results from the channel until either a 200 is found or every
	// handler has returned.
	for range m.handlers {
		select {
		case result := <-results:
			if result.err != nil {
				errs = append(errs, result.err)
				continue
			}

			if result.code == http.StatusOK {
				return http.StatusOK, nil
			}

			lastCode = result.code
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	if lastCode != 0 {
		return lastCode, nil
	}

	return 0, errors.Join(errs...)
}

// Post broadcasts a POST to all nodes and succeeds as soon as one node accepts.
// When resp is nil the response body is discarded.
func (m *multiHandler) Post(ctx context.Context, endpoint string, headers map[string]string, data *bytes.Buffer, resp any) error {
	// Short-circuit when there's only one handler.
	if len(m.handlers) == 1 {
		if err := m.handlers[0].Post(ctx, endpoint, headers, data, resp); err != nil {
			return fmt.Errorf("post: %w", err)
		}

		return nil
	}

	raw := []byte{}
	if data != nil {
		raw = data.Bytes()
	}

	post := func(ctx context.Context, h *handler) (json.RawMessage, error) {
		// We don't care about the response body if resp is nil.
		if resp == nil {
			if err := h.Post(ctx, endpoint, headers, cloneBuffer(data, raw), nil); err != nil {
				return nil, fmt.Errorf("post: %w", err)
			}

			return nil, nil
		}

		// We do care about the response body.
		cloned := cloneBuffer(data, raw)

		var out json.RawMessage
		if err := h.Post(ctx, endpoint, headers, cloned, &out); err != nil {
			return nil, fmt.Errorf("post: %w", err)
		}

		return out, nil
	}

	out, err := broadcastWrite(ctx, m.handlers, post)
	if err != nil {
		return fmt.Errorf("broadcastWrite: %w", err)
	}

	if err := decodeInto(out, resp); err != nil {
		return fmt.Errorf("decode into: %w", err)
	}

	return nil
}

// PostSSZ broadcasts an SSZ-preferred POST to all nodes and returns the first
// successful response.
func (m *multiHandler) PostSSZ(ctx context.Context, endpoint string, headers map[string]string, data *bytes.Buffer) ([]byte, http.Header, error) {
	if len(m.handlers) == 1 {
		result, header, err := m.handlers[0].PostSSZ(ctx, endpoint, headers, data)
		if err != nil {
			return nil, nil, fmt.Errorf("post ssz: %w", err)
		}

		return result, header, nil
	}

	var raw []byte
	if data != nil {
		raw = data.Bytes()
	}

	post := func(ctx context.Context, h *handler) (sszResult, error) {
		body, hdr, err := h.PostSSZ(ctx, endpoint, headers, cloneBuffer(data, raw))
		if err != nil {
			return sszResult{}, fmt.Errorf("post ssz: %w", err)
		}

		return sszResult{body: body, header: hdr}, nil
	}

	vals, errs := broadcastWriteAll(ctx, m.handlers, post)
	for _, err := range errs {
		if errors.Is(err, &httputil.DefaultJsonError{Code: http.StatusNotAcceptable}) {
			return nil, nil, err
		}
	}

	if len(vals) > 0 {
		return vals[0].body, vals[0].header, nil
	}

	return nil, nil, errors.Join(errs...)
}

// readUntil runs round against the handlers.
func readUntil[T any](
	ctx context.Context,
	handlers []*handler,
	cfg getConfig,
	accept func(T) bool,
	round readRound[T],
	fn queryFunc[T],
) (T, bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		zero     T
		fallback *T
	)

	for {
		// If specified, set a deadline.
		roundCtx, roundCancel := ctx, func() {}
		if !cfg.deadline.IsZero() {
			roundCtx, roundCancel = context.WithDeadline(ctx, cfg.deadline)
		}

		// Run a round of queries.
		val, matched, ok, errs := round(roundCtx, handlers, cfg.fallbackDeadline, accept, fn)
		roundCancel()

		// If a match was found, return it immediately.
		if matched {
			return val, true, nil
		}

		// If no match was found but a usable response was returned, record it as a
		// best-effort fallback.
		if ok {
			fallback = &val
		}

		// Stop after this round unless re-polling is enabled and there is still
		// time left on the deadline. In UntilAny2xx mode a usable fallback
		// also ends the re-polling.
		repollExhausted := ctx.Err() != nil || cfg.pollInterval <= 0 || cfg.deadline.IsZero() || !time.Now().Before(cfg.deadline)
		if cfg.repollMode == UntilAny2xx && fallback != nil {
			repollExhausted = true
		}

		if repollExhausted {
			if fallback != nil {
				return *fallback, false, nil
			}

			return zero, false, errors.Join(errs...)
		}

		// Wait for the poll interval to elapse.
		select {
		case <-ctx.Done():
			if fallback != nil {
				return *fallback, false, nil
			}

			return zero, false, ctx.Err()
		case <-time.After(cfg.pollInterval):
		}
	}
}

// roundFor selects the query strategy.
func roundFor[T any](cfg getConfig) readRound[T] {
	if cfg.race {
		return raceRound[T]
	}

	return inOrderRound[T]
}

// raceRound queries every handler concurrently and returns the first response
// satisfying accept.
//
// It returns four values:
//   - val: the chosen response
//   - matched: whether val satisfies the accept predicate.
//   - ok: whether val is a usable (2XX) response at all (false only when every
//     handler failed).
//   - errs: the per-handler failures collected this round (plus ctx.Err() when
//     the context was cancelled mid-run).
func raceRound[T any](ctx context.Context, handlers []*handler, fallbackDeadline time.Time, accept func(T) bool, fn queryFunc[T]) (T, bool, bool, []error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		val T
		err error
	}

	// Call fn concurrently and asynchronously on every handler, sending the result to results.
	results := make(chan result, len(handlers))
	for _, h := range handlers {
		go func(h *handler) {
			val, err := fn(ctx, h)
			results <- result{val: val, err: err}
		}(h)
	}

	var (
		fallback *T
		errs     []error
	)

	var fallbackExpiry <-chan time.Time

	// Pull results from the channel until either a match is found or every handler has returned.
	for range handlers {
		var r result
		select {
		case r = <-results:
		case <-fallbackExpiry:
			return *fallback, false, true, errs
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
			if fallback != nil {
				return *fallback, false, true, errs
			}

			var zero T
			return zero, false, false, errs
		}

		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}

		// If r.val satisfies accept, return it immediately.
		if accept(r.val) {
			return r.val, true, true, errs
		}

		if fallback == nil {
			fallback = &r.val

			if !fallbackDeadline.IsZero() {
				fallbackExpiry = time.After(time.Until(fallbackDeadline))
			}
		}
	}

	if fallback != nil {
		return *fallback, false, true, errs
	}

	var zero T
	return zero, false, false, errs
}

// inOrderRound tries handlers in sequence, stopping at the first accept match.
//
// It returns four values:
//   - val: the chosen response
//   - matched: whether val satisfies the accept predicate.
//   - ok: whether val is a usable (2XX) response at all (false only when every
//     handler failed).
//   - errs: the per-handler failures collected this round (plus ctx.Err() when
//     the context was cancelled mid-run).
func inOrderRound[T any](ctx context.Context, handlers []*handler, fallbackDeadline time.Time, accept func(T) bool, fn queryFunc[T]) (T, bool, bool, []error) {
	var (
		fallback *T
		errs     []error
	)

	for _, handler := range handlers {
		// Stop if the context is on error.
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}

		// With a usable response already in hand, stop querying the remaining
		// handlers once a better one is no longer worth waiting for, and bound
		// the queries still worth trying so a hung handler cannot outlast it.
		callCtx, cancel := ctx, func() {}
		if fallback != nil && !fallbackDeadline.IsZero() {
			if !time.Now().Before(fallbackDeadline) {
				break
			}

			callCtx, cancel = context.WithDeadline(ctx, fallbackDeadline)
		}

		// Run the query function.
		val, err := fn(callCtx, handler)
		cancel()
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// If val satisfies accept, return it immediately.
		if accept(val) {
			return val, true, true, errs
		}

		// If no fallback has been recorded yet, record this val as a best-effort fallback.
		if fallback == nil {
			fallback = &val
		}
	}

	if fallback != nil {
		return *fallback, false, true, errs
	}

	var zero T
	return zero, false, false, errs
}

// decodeInto unmarshals raw into resp, tolerating a nil resp or an empty body.
func decodeInto(raw json.RawMessage, resp any) error {
	if resp == nil || len(raw) == 0 {
		return nil
	}

	if err := json.Unmarshal(raw, resp); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	return nil
}

// broadcastWrite runs fn against every handler concurrently and returns the
// result of the first handler to succeed. If every handler fails, the joined
// error is returned.
func broadcastWrite[T any](ctx context.Context, handlers []*handler, fn func(context.Context, *handler) (T, error)) (T, error) {
	// Detach from the caller's cancellation so writes still reach every node after we return.
	bgCtx := context.WithoutCancel(ctx)

	// If the caller has a deadline, propagate it to the detached.
	cancel := func() {}
	if deadline, ok := ctx.Deadline(); ok {
		bgCtx, cancel = context.WithDeadline(bgCtx, deadline)
	}

	type result struct {
		val T
		err error
	}

	// Call fn concurrently and asynchronously on every handler, sending the result to results.
	var wg sync.WaitGroup
	results := make(chan result, len(handlers))
	for _, handler := range handlers {
		wg.Go(func() {
			val, err := fn(bgCtx, handler)
			results <- result{val: val, err: err}
		})
	}

	// Release the deadline timer once every detached write has finished.
	go func() {
		wg.Wait()
		cancel()
	}()

	// Collect results until either a success is found or every handler has returned.
	var errs []error
	for range handlers {
		select {
		case r := <-results:
			if r.err == nil {
				return r.val, nil
			}

			errs = append(errs, r.err)
		case <-ctx.Done():
			// Caller gave up waiting: drain results already in hand for a success
			// before returning, otherwise report the context error.
			for {
				select {
				case r := <-results:
					if r.err == nil {
						return r.val, nil
					}

					errs = append(errs, r.err)
				default:
					var zero T
					return zero, ctx.Err()
				}
			}
		}
	}

	var zero T
	return zero, errors.Join(errs...)
}

// broadcastWriteAll runs fn against every handler concurrently and waits for all
// of them, returning the successful values and the errors separately.
func broadcastWriteAll[T any](ctx context.Context, handlers []*handler, fn func(context.Context, *handler) (T, error)) ([]T, []error) {
	// Detach from the caller's cancellation so writes still reach every node after we return.
	bgCtx := context.WithoutCancel(ctx)

	// If the caller has a deadline, propagate it to the detached.
	cancel := func() {}
	if deadline, ok := ctx.Deadline(); ok {
		bgCtx, cancel = context.WithDeadline(bgCtx, deadline)
	}

	type result struct {
		val T
		err error
	}

	// Call fn concurrently and asynchronously on every handler, sending the result to results.
	var wg sync.WaitGroup
	results := make(chan result, len(handlers))
	for _, handler := range handlers {
		wg.Go(func() {
			val, err := fn(bgCtx, handler)
			results <- result{val: val, err: err}
		})
	}

	// Release the deadline timer once every detached write has finished.
	go func() {
		wg.Wait()
		cancel()
	}()

	var (
		vals []T
		errs []error
	)

	collect := func(r result) {
		if r.err != nil {
			errs = append(errs, r.err)
			return
		}

		vals = append(vals, r.val)
	}

	// Collect results until every handler has returned.
	for range handlers {
		select {
		case r := <-results:
			collect(r)
		case <-ctx.Done():
			// Caller gave up waiting: drain results already in hand for successes
			// before returning, otherwise report the context error.
			for {
				select {
				case r := <-results:
					collect(r)
				default:
					if len(vals) == 0 && len(errs) == 0 {
						errs = append(errs, ctx.Err())
					}

					return vals, errs
				}
			}
		}
	}

	return vals, errs
}

// cloneBuffer returns a fresh buffer over a copy of raw, or nil when the
// original data was nil.
func cloneBuffer(data *bytes.Buffer, raw []byte) *bytes.Buffer {
	if data == nil {
		return nil
	}

	return bytes.NewBuffer(bytes.Clone(raw))
}
