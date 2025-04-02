package client

import (
	"context"
	"time"

	"github.com/pkg/errors"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/math"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	octrace "go.opentelemetry.io/otel/trace"
)

// WaitForActivation checks whether the validator pubkey is in the active
// validator set. If not, this operation will block until an activation message is
// received. This method also monitors the keymanager for updates while waiting for an activation
// from the gRPC server.
//
// If the channel parameter is nil, WaitForActivation creates and manages its own channel.
func (v *validator) WaitForActivation(ctx context.Context, accountsChangedChan chan [][fieldparams.BLSPubkeyLength]byte) error {
	// Monitor the key manager for updates.
	if accountsChangedChan == nil {
		accountsChangedChan = make(chan [][fieldparams.BLSPubkeyLength]byte, 1)
		km, err := v.Keymanager()
		if err != nil {
			return err
		}
		// subscribe to the channel if it's the first time
		sub := km.SubscribeAccountChanges(accountsChangedChan)
		defer func() {
			sub.Unsubscribe()
			close(accountsChangedChan)
		}()
	}
	return v.internalWaitForActivation(ctx, accountsChangedChan)
}

// internalWaitForActivation recursively waits for at least one active validator key
func (v *validator) internalWaitForActivation(ctx context.Context, accountsChangedChan <-chan [][fieldparams.BLSPubkeyLength]byte) error {
	ctx, span := trace.StartSpan(ctx, "validator.WaitForActivation")
	defer span.End()

	// Step 1: Fetch validating public keys.
	validatingKeys, err := v.km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return errors.Wrap(err, msgCouldNotFetchKeys)
	}

	// Step 2: If no keys, wait for accounts change or context cancellation.
	if len(validatingKeys) == 0 {
		log.Warn(msgNoKeysFetched)
		return v.waitForAccountsChange(ctx, accountsChangedChan)
	}

	// Step 3: update validator statuses in cache.
	if err := v.updateValidatorStatusCache(ctx, validatingKeys); err != nil {
		return v.retryWaitForActivation(ctx, span, err, "Connection broken while waiting for activation. Reconnecting...", accountsChangedChan)
	}

	// Step 4: Check and log validator statuses.
	someAreActive := v.checkAndLogValidatorStatus()
	if !someAreActive {
		// Step 6: If no active validators, wait for accounts change, context cancellation, or next epoch.
		select {
		case <-ctx.Done():
			log.Debug("Context closed, exiting WaitForActivation")
			return ctx.Err()
		case <-accountsChangedChan:
			// Accounts (keys) changed, restart the process.
			return v.internalWaitForActivation(ctx, accountsChangedChan)
		default:
			if err := v.waitForNextEpoch(ctx, v.genesisTime, accountsChangedChan); err != nil {
				return v.retryWaitForActivation(ctx, span, err, "Failed to wait for next epoch. Reconnecting...", accountsChangedChan)
			}
			return v.internalWaitForActivation(incrementRetries(ctx), accountsChangedChan)
		}
	}
	return nil
}

func (v *validator) retryWaitForActivation(ctx context.Context, span octrace.Span, err error, message string, accountsChangedChan <-chan [][fieldparams.BLSPubkeyLength]byte) error {
	tracing.AnnotateError(span, err)
	attempts := activationAttempts(ctx)
	log.WithError(err).WithField("attempts", attempts).Error(message)
	// Reconnection attempt backoff, up to 60s.
	time.Sleep(time.Second * time.Duration(math.Min(uint64(attempts), 60)))
	// TODO: refactor this to use the health tracker instead for reattempt
	return v.internalWaitForActivation(incrementRetries(ctx), accountsChangedChan)
}

func (v *validator) waitForAccountsChange(ctx context.Context, accountsChangedChan <-chan [][fieldparams.BLSPubkeyLength]byte) error {
	select {
	case <-ctx.Done():
		log.Debug("Context closed, exiting waitForAccountsChange")
		return ctx.Err()
	case <-accountsChangedChan:
		// If the accounts changed, try again.
		return v.internalWaitForActivation(ctx, accountsChangedChan)
	}
}

// waitForNextEpoch creates a blocking function to wait until the next epoch start given the current slot
func (v *validator) waitForNextEpoch(ctx context.Context, genesisTimeSec uint64, accountsChangedChan <-chan [][fieldparams.BLSPubkeyLength]byte) error {
	waitTime, err := slots.SecondsUntilNextEpochStart(genesisTimeSec)
	if err != nil {
		return err
	}
	log.WithField("seconds_until_next_epoch", waitTime).Warn("No active validator keys provided. Waiting until next epoch to check again...")
	select {
	case <-ctx.Done():
		log.Debug("Context closed, exiting waitForNextEpoch")
		return ctx.Err()
	case <-accountsChangedChan:
		// Accounts (keys) changed, restart the process.
		return v.internalWaitForActivation(ctx, accountsChangedChan)
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
