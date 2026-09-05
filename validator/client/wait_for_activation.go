package client

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
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

// waitForActivation implements WaitForActivation. accountsChanged marks passes
// entered after a key change, whose keys HandleKeyReload never saw.
func (v *validator) waitForActivation(ctx context.Context, accountsChanged bool) error {
	ctx, span := trace.StartSpan(ctx, "validator.WaitForActivation")
	defer span.End()

	for {
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
			if err := v.waitForAccountsChange(ctx); err != nil {
				return errors.Wrap(err, "could not wait for accounts change")
			}
			accountsChanged = true
			continue
		}

		// Step 3: update validator statuses in cache, waiting out a broken connection.
		if err := v.updateValidatorStatusCache(ctx, validatingKeys); err != nil {
			if ctx.Err() != nil {
				return errors.Wrap(ctx.Err(), "context closed while waiting for activation")
			}
			tracing.AnnotateError(span, err)
			log.WithError(err).Error("Connection broken while waiting for activation. Reconnecting...")
			if err := v.healthMonitor.WaitForHealthy(ctx); err != nil {
				return errors.Wrap(err, "could not wait for a healthy beacon node")
			}
			continue
		}

		// Step 4: Check and log validator statuses.
		if v.checkAndLogValidatorStatus() {
			return nil
		}

		// Step 5: If no active validators, wait for accounts change, context cancellation, or next epoch.
		changed, err := v.waitForNextEpoch(ctx, v.genesisTime)
		if err != nil {
			return errors.Wrap(err, "could not wait for next epoch")
		}
		if changed {
			accountsChanged = true
		}
	}
}

// waitForAccountsChange blocks until the keymanager reports a key change.
func (v *validator) waitForAccountsChange(ctx context.Context) error {
	select {
	case <-ctx.Done():
		log.Debug("Context closed, exiting waitForAccountsChange")
		return ctx.Err()
	case <-v.accountsChangedChannel:
		return nil
	}
}

// waitForNextEpoch blocks until the next epoch starts, or returns true as soon
// as the accounts change instead.
func (v *validator) waitForNextEpoch(ctx context.Context, genesis time.Time) (bool, error) {
	waitTime, err := slots.SecondsUntilNextEpochStart(genesis)
	if err != nil {
		return false, errors.Wrap(err, "could not compute time until next epoch")
	}
	log.WithField("seconds_until_next_epoch", waitTime).Warn("No active validator keys provided. Waiting until next epoch to check again...")
	select {
	case <-ctx.Done():
		log.Debug("Context closed, exiting waitForNextEpoch")
		return false, ctx.Err()
	case <-v.accountsChangedChannel:
		return true, nil
	case <-time.After(time.Duration(waitTime) * time.Second):
		log.Debug("Done waiting for epoch start")
		return false, nil
	}
}
