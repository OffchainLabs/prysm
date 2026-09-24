package rest

import (
	"context"
	"errors"
	"time"
)

type independentQueryResult[T any] struct {
	index int
	val   T
	err   error
}

func queryIndependentlyUntilAccepted[T any](ctx context.Context, handlers []*handler, cfg queryConfig, accept func(T) bool, fn queryFunc[T]) (T, bool, error) {
	queryCtx, cancel := context.WithDeadline(ctx, cfg.deadline)
	defer cancel()

	results := make(chan independentQueryResult[T], len(handlers))
	for i, h := range handlers {
		go independentlyPollHandler(queryCtx, i, h, cfg, fn, results)
	}

	var fallback *T
	lastErrors := make([]error, len(handlers))
	for {
		select {
		case r := <-results:
			if r.err != nil {
				if queryCtx.Err() == nil || !errors.Is(r.err, queryCtx.Err()) {
					lastErrors[r.index] = r.err
				}
				continue
			}
			if accept(r.val) {
				return r.val, true, nil
			}
			fallback = &r.val
		case <-queryCtx.Done():
			return finishIndependentQuery(ctx, queryCtx, fallback, lastErrors)
		}
	}
}

func independentlyPollHandler[T any](ctx context.Context, index int, h *handler, cfg queryConfig, fn queryFunc[T], results chan<- independentQueryResult[T]) {
	for ctx.Err() == nil {
		val, err := fn(ctx, h)
		select {
		case results <- independentQueryResult[T]{index: index, val: val, err: err}:
		case <-ctx.Done():
			return
		}
		select {
		case <-time.After(cfg.pollInterval):
		case <-ctx.Done():
			return
		}
	}
}

func finishIndependentQuery[T any](ctx, queryCtx context.Context, fallback *T, lastErrors []error) (T, bool, error) {
	if fallback != nil {
		return *fallback, false, nil
	}
	var zero T
	err := errors.Join(lastErrors...)
	if ctx.Err() != nil {
		return zero, false, errors.Join(err, ctx.Err())
	}
	if err == nil {
		err = queryCtx.Err()
	}
	return zero, false, err
}
