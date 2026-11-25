package kurtosis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/services"
	"github.com/kurtosis-tech/kurtosis/api/golang/engine/lib/kurtosis_context"
)

const logSearchInterval = 2 * time.Second // interval between grep attempts for logs.

// WaitForServiceLog waits until the named service logs the given substring.
func (kw *KurtosisWrapper) WaitForServiceLog(ctx context.Context, serviceName, substring string, timeout time.Duration) error {
	if kw.enclaveCtx == nil {
		return errors.New("enclave context is nil")
	}
	if serviceName == "" {
		return errors.New("service name is required")
	}
	if substring == "" {
		return errors.New("log substring is required")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastErr error
	for {
		found, err := kw.serviceLogContains(ctx, serviceName, substring)
		if found {
			return nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for %q in logs for %q: %w", substring, serviceName, lastErr)
			}
			return fmt.Errorf("timed out waiting for %q in logs for %q", substring, serviceName)
		}
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("context canceled waiting for %q in logs for %q: %w", substring, serviceName, err)
		}
		if err != nil {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for %q in logs for %q: %w", substring, serviceName, lastErr)
			}
			return fmt.Errorf("timed out waiting for %q in logs for %q", substring, serviceName)
		case <-time.After(logSearchInterval):
		}
	}
}

// serviceLogContains follows the Kurtosis log stream with a server-side substring filter.
func (kw *KurtosisWrapper) serviceLogContains(ctx context.Context, serviceName, substring string) (bool, error) {
	svc, err := kw.enclaveCtx.GetServiceContext(serviceName)
	if err != nil {
		return false, fmt.Errorf("get service %q: %w", serviceName, err)
	}

	svcUUID := svc.GetServiceUUID()

	logs, stop, err := kw.kurtosisCtx.GetServiceLogs(
		ctx,
		kw.enclaveName,
		map[services.ServiceUUID]bool{svcUUID: true},
		true, // shouldFollowLogs = true.
		true, // shouldReturnAllLogs = true.
		0,
		kurtosis_context.NewDoesContainTextLogLineFilter(substring),
	)
	if err != nil {
		return false, fmt.Errorf("get logs for %q: %w", serviceName, err)
	}
	defer stop()

	for chunk := range logs {
		if _, ok := chunk.GetNotFoundServiceUuids()[svcUUID]; ok {
			return false, fmt.Errorf("logs for service %q not found", serviceName)
		}
		if len(chunk.GetServiceLogsByServiceUuids()[svcUUID]) > 0 {
			return true, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context error for service %q: %w", serviceName, err)
	}
	return false, nil
}
