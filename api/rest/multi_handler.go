package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// multiHandler is a rest.Handler that fans every request out across multiple
// beacon node endpoints ("active-active"). Reads (GET) race all nodes and return
// the first successful response, cancelling the other in-flight requests once one
// wins. Writes (POST) are broadcast to every node and return as soon as one node
// accepts.
type multiHandler struct {
	handlers []*handler
}

// newMultiHandler builds a multiHandler over the given per-endpoint handlers.
func newMultiHandler(handlers []*handler) (*multiHandler, error) {
	if len(handlers) == 0 {
		return nil, errors.New("multiHandler requires at least one handler")
	}

	return &multiHandler{handlers: handlers}, nil
}

// Host returns a representative endpoint (the first) for logging purposes.
func (m *multiHandler) Host() string {
	return m.handlers[0].Host()
}

// Get races a GET across all nodes and decodes the first successful response
// into resp.
func (m *multiHandler) Get(ctx context.Context, endpoint string, resp any) error {
	if len(m.handlers) == 1 {
		if err := m.handlers[0].Get(ctx, endpoint, resp); err != nil {
			return fmt.Errorf("get: %w", err)
		}

		return nil
	}

	get := func(ctx context.Context, h *handler) (json.RawMessage, error) {
		if resp == nil {
			if err := h.Get(ctx, endpoint, nil); err != nil {
				return nil, fmt.Errorf("get: %w", err)
			}

			return nil, nil
		}

		var raw json.RawMessage
		if err := h.Get(ctx, endpoint, &raw); err != nil {
			return nil, fmt.Errorf("get: %w", err)
		}

		return raw, nil
	}

	raw, err := raceRead(ctx, m.handlers, get)
	if err != nil {
		return fmt.Errorf("raceRead: %w", err)
	}

	if resp == nil || len(raw) == 0 {
		return nil
	}

	if err := json.Unmarshal(raw, resp); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	return nil
}

// GetStatusCode queries all nodes and prefers a 200 (ready): it returns 200 if
// any node is fully synced, otherwise the last non-200 status observed, or a
// joined error if every node failed at the transport level.
func (m *multiHandler) GetStatusCode(ctx context.Context, endpoint string) (int, error) {
	if len(m.handlers) == 1 {
		statusCode, err := m.handlers[0].GetStatusCode(ctx, endpoint)
		if err != nil {
			return 0, fmt.Errorf("get status code: %w", err)
		}

		return statusCode, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		code int
		err  error
	}

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

	for range m.handlers {
		result := <-results
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}

		if result.code == http.StatusOK {
			return http.StatusOK, nil
		}

		lastCode = result.code
	}

	if lastCode != 0 {
		return lastCode, nil
	}

	return 0, errors.Join(errs...)
}

// GetSSZ races a GET (SSZ-preferred) across all nodes and returns the first
// successful response.
func (m *multiHandler) GetSSZ(ctx context.Context, endpoint string) ([]byte, http.Header, error) {
	if len(m.handlers) == 1 {
		resp, header, err := m.handlers[0].GetSSZ(ctx, endpoint)
		if err != nil {
			return nil, nil, fmt.Errorf("get ssz: %w", err)
		}
		return resp, header, nil
	}

	type sszResult struct {
		body []byte
		hdr  http.Header
	}

	get := func(ctx context.Context, h *handler) (sszResult, error) {
		body, header, err := h.GetSSZ(ctx, endpoint)
		if err != nil {
			return sszResult{}, fmt.Errorf("get ssz: %w", err)
		}

		return sszResult{body: body, hdr: header}, nil
	}

	res, err := raceRead(ctx, m.handlers, get)
	if err != nil {
		return nil, nil, err
	}

	return res.body, res.hdr, nil
}

// Post broadcasts a POST to all nodes and succeeds as soon as one node accepts.
// The first successful response is decoded into resp.
func (m *multiHandler) Post(ctx context.Context, endpoint string, headers map[string]string, data *bytes.Buffer, resp any) error {
	if len(m.handlers) == 1 {
		if err := m.handlers[0].Post(ctx, endpoint, headers, data, resp); err != nil {
			return fmt.Errorf("post: %w", err)
		}

		return nil
	}

	raw := snapshot(data)
	post := func(ctx context.Context, h *handler) (json.RawMessage, error) {
		if resp == nil {
			if err := h.Post(ctx, endpoint, headers, cloneBuffer(data, raw), nil); err != nil {
				return nil, fmt.Errorf("post: %w", err)
			}

			return nil, nil
		}

		var out json.RawMessage
		if err := h.Post(ctx, endpoint, headers, cloneBuffer(data, raw), &out); err != nil {
			return nil, fmt.Errorf("post: %w", err)
		}

		return out, nil
	}

	out, err := broadcastWrite(ctx, m.handlers, post)
	if err != nil {
		return fmt.Errorf("broadcastWrite: %w", err)
	}

	if resp == nil || len(out) == 0 {
		return nil
	}

	if err := json.Unmarshal(out, resp); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
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

	raw := snapshot(data)
	type sszResult struct {
		body []byte
		hdr  http.Header
	}

	post := func(ctx context.Context, h *handler) (sszResult, error) {
		body, hdr, err := h.PostSSZ(ctx, endpoint, headers, cloneBuffer(data, raw))
		if err != nil {
			return sszResult{}, fmt.Errorf("post ssz: %w", err)
		}

		return sszResult{body: body, hdr: hdr}, nil
	}

	res, err := broadcastWrite(ctx, m.handlers, post)
	if err != nil {
		return nil, nil, err
	}

	return res.body, res.hdr, nil
}

// raceRead runs fn against every handler concurrently and returns the result of
// the first handler to succeed (nil error), cancelling the others. If every
// handler fails, the joined error is returned.
func raceRead[T any](ctx context.Context, handlers []*handler, fn func(context.Context, *handler) (T, error)) (T, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		val T
		err error
	}

	results := make(chan result, len(handlers))

	for _, h := range handlers {
		go func(h *handler) {
			val, err := fn(ctx, h)
			results <- result{val: val, err: err}
		}(h)
	}

	var errs []error
	for range handlers {
		result := <-results
		if result.err == nil {
			return result.val, nil
		}

		errs = append(errs, result.err)
	}

	var zero T
	return zero, errors.Join(errs...)
}

// broadcastWrite runs fn against every handler concurrently and returns the
// result of the first handler to succeed. If every handler fails, the joinded
// error is returned.
func broadcastWrite[T any](ctx context.Context, handlers []*handler, fn func(context.Context, *handler) (T, error)) (T, error) {
	bgCtx := context.WithoutCancel(ctx)

	type result struct {
		val T
		err error
	}

	results := make(chan result, len(handlers))
	for _, h := range handlers {
		go func(h *handler) {
			val, err := fn(bgCtx, h)
			results <- result{val: val, err: err}
		}(h)
	}

	var errs []error
	for range handlers {
		r := <-results
		if r.err == nil {
			return r.val, nil
		}

		errs = append(errs, r.err)
	}

	var zero T
	return zero, errors.Join(errs...)
}

// snapshot returns the bytes backing data, or nil when data is nil.
func snapshot(data *bytes.Buffer) []byte {
	if data == nil {
		return nil
	}

	return data.Bytes()
}

// cloneBuffer returns a fresh buffer over a copy of raw, or nil when the
// original data was nil.
func cloneBuffer(data *bytes.Buffer, raw []byte) *bytes.Buffer {
	if data == nil {
		return nil
	}

	return bytes.NewBuffer(bytes.Clone(raw))
}
