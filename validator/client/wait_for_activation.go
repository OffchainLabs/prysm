package client

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	octrace "go.opentelemetry.io/otel/trace"
)

// WaitForActivation checks whether the validator pubkey is in the active
// validator set. If not, this operation will block until an activation message is
// received. This method also monitors the keymanager for updates while waiting for an activation
// from the gRPC server.
//
// If the channel parameter is nil, WaitForActivation creates and manages its own channel.
func (v *validator) WaitForActivation(ctx context.Context) error {
	return v.waitForActivation(ctx, false)
}

// waitForActivation implements WaitForActivation. accountsChanged marks calls
// re-entered after a key change, whose keys HandleKeyReload never saw.
func (v *validator) waitForActivation(ctx context.Context, accountsChanged bool) error {
	ctx, span := trace.StartSpan(ctx, "validator.WaitForActivation")
	defer span.End()

	// Step 1: Fetch validating public keys.
	validatingKeys, err := v.km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return errors.Wrap(err, msgCouldNotFetchKeys)
	}

	// Startup keys are vetted by the startup doppelganger check instead.
	if accountsChanged {
		v.trackReloadedKeysForDoppelGanger(validatingKeys)
	}

	// Step 2: If no keys, wait for accounts change or context cancellation.
	if len(validatingKeys) == 0 {
		log.Warn(msgNoKeysFetched)
		return v.waitForAccountsChange(ctx)
	}

	// Step 3: update validator statuses in cache.
	if err := v.updateValidatorStatusCache(ctx, validatingKeys); err != nil {
		return v.retryWaitForActivation(ctx, span, err, "Connection broken while waiting for activation. Reconnecting...", accountsChanged)
	}

	// Step 4: Check and log validator statuses.
	someAreActive := v.checkAndLogValidatorStatus()
	if !someAreActive {
		// Step 6: If no active validators, wait for accounts change, context cancellation, or next epoch.
		select {
		case <-ctx.Done():
			log.Debug("Context closed, exiting WaitForActivation")
			return ctx.Err()
		case <-v.accountsChangedChannel:
			// Accounts (keys) changed, restart the process.
			return v.waitForActivation(ctx, true)
		default:
			if err := v.waitForNextEpoch(ctx, v.genesisTime); err != nil {
				return v.retryWaitForActivation(ctx, span, err, "Failed to wait for next epoch. Reconnecting...", accountsChanged)
			}
			return v.waitForActivation(incrementRetries(ctx), accountsChanged)
		}
	}
	return nil
}

func (v *validator) retryWaitForActivation(ctx context.Context, span octrace.Span, err error, message string, accountsChanged bool) error {
	tracing.AnnotateError(span, err)
	attempts := activationAttempts(ctx)
	log.WithError(err).WithField("attempts", attempts).Error(message)
	// Reconnection attempt backoff, up to 60s.
	time.Sleep(time.Second * time.Duration(min(uint64(attempts), 60)))
	// TODO: refactor this to use the health tracker instead for reattempt
	return v.waitForActivation(incrementRetries(ctx), accountsChanged)
}

func (v *validator) waitForAccountsChange(ctx context.Context) error {
	select {
	case <-ctx.Done():
		log.Debug("Context closed, exiting waitForAccountsChange")
		return ctx.Err()
	case <-v.accountsChangedChannel:
		// If the accounts changed, try again.
		return v.waitForActivation(ctx, true)
	}
}

// waitForNextEpoch creates a blocking function to wait until the next epoch start given the current slot
func (v *validator) waitForNextEpoch(ctx context.Context, genesis time.Time) error {
	waitTime, err := slots.SecondsUntilNextEpochStart(genesis)
	if err != nil {
		return err
	}
	log.WithField("seconds_until_next_epoch", waitTime).Warn("No active validator keys provided. Waiting until next epoch to check again...")
	select {
	case <-ctx.Done():
		log.Debug("Context closed, exiting waitForNextEpoch")
		return ctx.Err()
	case <-v.accountsChangedChannel:
		// Accounts (keys) changed, restart the process.
		return v.waitForActivation(ctx, true)
	case <-time.After(time.Duration(waitTime) * time.Second):
		log.Debug("Done waiting for epoch start")
		// The ticker has ticked, indicating we've reached the next epoch
		return nil
	}
}

// Preferred way to use context keys is with a non built-in type. See: RVV-B0003
type waitForActivationContextKey string

const waitForActivationAttemptsContextKey = waitForActivationContextKey("WaitForActivation-attempts")

func activationAttempts(ctx context.Context) int {
	attempts, ok := ctx.Value(waitForActivationAttemptsContextKey).(int)
	if !ok {
		return 1
	}
	return attempts
}

func incrementRetries(ctx context.Context) context.Context {
	attempts := activationAttempts(ctx)
	return context.WithValue(ctx, waitForActivationAttemptsContextKey, attempts+1)
}
