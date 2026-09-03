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

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
)

const sszUnsupportedTTL = 24 * time.Hour

type (
	multiHandler struct {
		handlers       []*handler
		sszUnsupported sync.Map // sszSupportKey -> time.Time
	}

	sszSupportKey struct {
		host     string
		endpoint string
	}

	sszResult struct {
		body   []byte
		header http.Header
	}

	queryFunc[T any] func(context.Context, *handler) (T, error)

	// queryRound queries every handler once and reports the outcome:
	//   - matched=true: val is the first response satisfying accept.
	//   - matched=false, ok=true: no response matched, val is a best-effort success.
	//   - matched=false, ok=false: every handler failed; errs holds the failures.
	queryRound[T any] func(ctx context.Context, handlers []*handler, fallbackDeadline time.Time, accept func(T) bool, fn queryFunc[T]) (val T, matched, ok bool, errs []error)
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
func (m *multiHandler) Get(ctx context.Context, endpoint string, resp any, opts ...QueryOption) error {
	cfg := newQueryConfig(opts)

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
	raw, _, err := queryUntilAccepted(ctx, m.handlers, cfg, cfg.accept, queryFunc, get)
	if err != nil {
		return fmt.Errorf("query until accepted: %w", err)
	}

	// Decode the response into resp.
	if err := decodeInto(raw, resp); err != nil {
		return fmt.Errorf("decode into: %w", err)
	}

	return nil
}

// GetSSZ reads a GET from the nodes, requesting SSZ but accepting JSON.
func (m *multiHandler) GetSSZ(ctx context.Context, endpoint string, opts ...QueryOption) ([]byte, http.Header, error) {
	get := func(ctx context.Context, h *handler) (sszResult, error) {
		body, header, err := h.GetSSZ(ctx, endpoint)
		if err != nil {
			return sszResult{}, fmt.Errorf("get ssz: %w", err)
		}

		return sszResult{body: body, header: header}, nil
	}

	return m.querySSZ(ctx, newQueryConfig(opts), get)
}

// querySSZ runs a per-node request through the query engine — racing, acceptance
// predicate, deadlines, and re-polling all come from cfg — and returns the winning
// response's body and headers.
func (m *multiHandler) querySSZ(ctx context.Context, cfg queryConfig, fn queryFunc[sszResult]) ([]byte, http.Header, error) {
	// Adapt the config's (body, header) predicate to the sszResult the rounds produce.
	accept := func(r sszResult) bool { return cfg.sszAccept(r.body, r.header) }

	res, _, err := queryUntilAccepted(ctx, m.handlers, cfg, accept, roundFor[sszResult](cfg), fn)
	if err != nil {
		return nil, nil, fmt.Errorf("query until accepted: %w", err)
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

// PostSSZ broadcasts a POST with an SSZ request body to all nodes, succeeding as soon as
// one accepts. A 415 from any node surfaces so the caller can re-broadcast via JSON.
func (m *multiHandler) PostSSZ(ctx context.Context, endpoint string, headers map[string]string, data *bytes.Buffer) error {
	if len(m.handlers) == 1 {
		if err := m.handlers[0].PostSSZ(ctx, endpoint, headers, data); err != nil {
			return fmt.Errorf("post ssz: %w", err)
		}

		return nil
	}

	var raw []byte
	if data != nil {
		raw = data.Bytes()
	}

	post := func(ctx context.Context, h *handler) error {
		if err := h.PostSSZ(ctx, endpoint, headers, cloneBuffer(data, raw)); err != nil {
			return fmt.Errorf("post ssz: %w", err)
		}

		return nil
	}

	accepted, errs := broadcastWriteAll(ctx, m.handlers, post)
	for _, err := range errs {
		if errors.Is(err, &httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType}) {
			return err
		}
	}

	if accepted > 0 {
		return nil
	}

	return errors.Join(errs...)
}

// PostSSZWithFallback broadcasts a POST to every node using each node's best known
// encoding, succeeding as soon as one node accepts. A 415 only downgrades that node to JSON.
func (m *multiHandler) PostSSZWithFallback(
	ctx context.Context,
	endpoint string,
	headers map[string]string,
	sszFn func() ([]byte, error),
	jsonFn func() ([]byte, error),
) error {
	post := m.postWithFallbackFn(endpoint, headers, sszFn, jsonFn)

	if len(m.handlers) == 1 {
		_, err := post(ctx, m.handlers[0])
		return err
	}

	accepted, errs := broadcastWriteAll(ctx, m.handlers, func(ctx context.Context, h *handler) error {
		_, err := post(ctx, h)
		return err
	})
	if accepted > 0 {
		return nil
	}
	return errors.Join(errs...)
}

// RequestSSZWithFallback posts through the read-query machinery (racing, acceptance
// predicates and deadlines via opts), preferring an SSZ response, and returns the winner.
func (m *multiHandler) RequestSSZWithFallback(
	ctx context.Context,
	endpoint string,
	headers map[string]string,
	sszFn func() ([]byte, error),
	jsonFn func() ([]byte, error),
	opts ...QueryOption,
) ([]byte, http.Header, error) {
	return m.querySSZ(ctx, newQueryConfig(opts), m.postWithFallbackFn(endpoint, headers, sszFn, jsonFn, withSSZResponse()))
}

// postWithFallbackFn returns the per-node post function using the node's best known
// encoding, downgrading to JSON after a 415; each marshaler runs at most once across nodes.
func (m *multiHandler) postWithFallbackFn(endpoint string, headers map[string]string, sszFn, jsonFn func() ([]byte, error), respOpts ...reqOption) queryFunc[sszResult] {
	sszBody := sync.OnceValues(sszFn)
	jsonBody := sync.OnceValues(jsonFn)

	return func(ctx context.Context, h *handler) (sszResult, error) {
		key := sszSupportKey{host: h.Host(), endpoint: memoEndpoint(endpoint)}
		if m.sszWorthTrying(key) {
			body, err := sszBody()
			if err != nil {
				return sszResult{}, fmt.Errorf("marshal SSZ body: %w", err)
			}

			data, header, err := h.postWithContentType(ctx, endpoint, headers, api.OctetStreamMediaType, bytes.NewBuffer(body), respOpts...)
			if !errors.Is(err, &httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType}) {
				if err != nil {
					return sszResult{}, fmt.Errorf("post SSZ: %w", err)
				}
				return sszResult{body: data, header: header}, nil
			}
			m.markSSZUnsupported(key, err)
		}

		body, err := jsonBody()
		if err != nil {
			return sszResult{}, fmt.Errorf("marshal JSON body: %w", err)
		}

		data, header, err := h.postWithContentType(ctx, endpoint, headers, api.JsonMediaType, bytes.NewBuffer(body), respOpts...)
		if err != nil {
			return sszResult{}, fmt.Errorf("post JSON: %w", err)
		}
		return sszResult{body: data, header: header}, nil
	}
}

// sszWorthTrying reports whether SSZ should be attempted for the node and endpoint.
func (m *multiHandler) sszWorthTrying(key sszSupportKey) bool {
	unsupportedAt, unsupported := m.sszUnsupported.Load(key)
	return !unsupported || time.Since(unsupportedAt.(time.Time)) >= sszUnsupportedTTL
}

// markSSZUnsupported memoizes a 415, logging only the first time for the key.
func (m *multiHandler) markSSZUnsupported(key sszSupportKey, err error) {
	if _, loaded := m.sszUnsupported.Swap(key, time.Now()); !loaded {
		log.WithError(err).
			WithField("host", api.RedactEndpoint(key.host)).
			WithField("endpoint", key.endpoint).
			Warn("Beacon node does not accept SSZ request bodies, falling back to JSON")
	}
}

// memoEndpoint reduces an endpoint to a stable memo key: the query string and
// numeric path segments (e.g. slots) vary per call and would fragment the memo.
func memoEndpoint(endpoint string) string {
	if i := strings.IndexByte(endpoint, '?'); i >= 0 {
		endpoint = endpoint[:i]
	}
	segments := strings.Split(endpoint, "/")
	for i, segment := range segments {
		if segment != "" && strings.IndexFunc(segment, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
			segments[i] = "*"
		}
	}
	return strings.Join(segments, "/")
}

// queryUntilAccepted runs rounds of queries against the handlers until a response
// satisfies accept, re-polling per cfg and falling back to the best 2XX seen.
func queryUntilAccepted[T any](
	ctx context.Context,
	handlers []*handler,
	cfg queryConfig,
	accept func(T) bool,
	round queryRound[T],
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
func roundFor[T any](cfg queryConfig) queryRound[T] {
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
// of them, returning the accepted count and the errors separately.
func broadcastWriteAll(ctx context.Context, handlers []*handler, fn func(context.Context, *handler) error) (uint16, []error) {
	// Detach from the caller's cancellation so writes still reach every node after we return.
	bgCtx := context.WithoutCancel(ctx)

	// If the caller has a deadline, propagate it to the detached.
	cancel := func() {}
	if deadline, ok := ctx.Deadline(); ok {
		bgCtx, cancel = context.WithDeadline(bgCtx, deadline)
	}

	// Call fn concurrently and asynchronously on every handler, sending the result to results.
	var wg sync.WaitGroup
	results := make(chan error, len(handlers))
	for _, handler := range handlers {
		wg.Go(func() {
			results <- fn(bgCtx, handler)
		})
	}

	// Release the deadline timer once every detached write has finished.
	go func() {
		wg.Wait()
		cancel()
	}()

	var (
		accepted uint16
		errs     []error
	)

	collect := func(err error) {
		if err != nil {
			errs = append(errs, err)
			return
		}

		accepted++
	}

	// Collect results until every handler has returned.
	for range handlers {
		select {
		case err := <-results:
			collect(err)
		case <-ctx.Done():
			// Caller gave up waiting: drain results already in hand for successes
			// before returning, otherwise report the context error.
			for {
				select {
				case err := <-results:
					collect(err)
				default:
					if accepted == 0 && len(errs) == 0 {
						errs = append(errs, ctx.Err())
					}

					return accepted, errs
				}
			}
		}
	}

	return accepted, errs
}

// cloneBuffer returns a fresh buffer over a copy of raw, or nil when the
// original data was nil.
func cloneBuffer(data *bytes.Buffer, raw []byte) *bytes.Buffer {
	if data == nil {
		return nil
	}

	return bytes.NewBuffer(bytes.Clone(raw))
}
