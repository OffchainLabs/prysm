package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/api/client"
	"github.com/OffchainLabs/prysm/v6/api/client/event"
	"github.com/OffchainLabs/prysm/v6/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	prysmTrace "github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	slotDeadlineThreshold = 2 * time.Second
)

// Run the main validator routine. This routine exits if the context is
// canceled.
//
// Order of operations:
// 1 - Init validator data
// 2 - Wait for validator activation
// 3 - Wait for the next slot start
// 4 - Update assignments
// 5 - Determine role at current slot
// 6 - Perform assigned role, if any
func run(ctx context.Context, v iface.Validator) error {
	cleanup := v.Done
	defer cleanup()

	if err := v.Init(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil // Exit if context is canceled.
		}
		return errors.Wrap(err, "failed to initialize validator")
	}
	tracker := v.HealthTracker()
	runHealthCheckRoutine(ctx, v)
	genesisTime, err := v.GenesisTime()
	if err != nil {
		return errors.Wrap(err, "failed to get genesis time")
	}
	for {
		select {
		case <-ctx.Done():
			log.Info("Context canceled, stopping validator")
			return nil // Exit if context is canceled.
		case slot := <-v.NextSlot():
			if !tracker.IsHealthy(ctx) {
				continue
			}
			deadline := v.SlotDeadline(slot)
			if time.Until(deadline) < slotDeadlineThreshold {
				log.WithFields(logrus.Fields{
					"slot":                slot,
					"current_slot":        slots.CurrentSlot(genesisTime),
					"time_until_deadline": time.Until(deadline),
					"deadline":            deadline,
					"threshold":           slotDeadlineThreshold,
				}).Debug("Skipping slot duties, too close or past deadline")
				continue
			}

			slotCtx, cancel := context.WithDeadline(ctx, deadline)

			var span trace.Span
			slotCtx, span = prysmTrace.StartSpan(slotCtx, "validator.processSlot")
			span.SetAttributes(prysmTrace.Int64Attribute("slot", int64(slot))) // lint:ignore uintcast -- This conversion is OK for tracing.

			log := log.WithField("slot", slot)
			log.WithField("deadline", deadline).Debug("Set deadline for proposals and attestations")

			// Keep trying to update assignments if they are nil or if we are past an
			// epoch transition in the beacon node's state.
			if slots.IsEpochStart(slot) {
				if err := v.UpdateDuties(slotCtx); err != nil {
					handleAssignmentError(err, slot)
					span.End()
					cancel()
					continue
				}
			}

			// call push proposer settings often to account for the following edge cases:
			// proposer is activated at the start of epoch and tries to propose immediately
			// account has changed in the middle of an epoch
			if err := v.PushProposerSettings(slotCtx, slot, false); err != nil {
				log.WithError(err).Warn("Failed to update proposer settings")
			}

			// Start fetching domain data for the next epoch.
			if slots.IsEpochEnd(slot) {
				go v.UpdateDomainDataCaches(slotCtx, slot+1)
			}

			var wg sync.WaitGroup

			allRoles, err := v.RolesAt(slotCtx, slot)
			if err != nil {
				log.WithError(err).Error("Could not get validator roles")
				span.End()
				cancel()
				continue
			}
			performRoles(slotCtx, allRoles, v, slot, &wg, span)
		case isHealthyAgain := <-tracker.HealthUpdates():
			if isHealthyAgain {
				if err := v.Init(ctx); err != nil {
					if errors.Is(err, context.Canceled) {
						return nil // Exit if context is canceled.
					}
					return errors.Wrap(err, "failed to re-initialize validator")
				}
			}
		case e := <-v.EventsChan():
			v.ProcessEvent(ctx, e)
		case currentKeys := <-v.AccountsChangedChan(): // should be less of a priority than next slot
			onAccountsChanged(ctx, v, currentKeys)
		}
	}
}

func onAccountsChanged(ctx context.Context, v iface.Validator, current [][48]byte) {
	ctx, span := prysmTrace.StartSpan(ctx, "validator.accountsChanged")
	defer span.End()

	anyActive, err := v.HandleKeyReload(ctx, current)
	if err != nil {
		log.WithError(err).Error("Could not properly handle reloaded keys")
	}
	if !anyActive {
		log.Warn("No active keys found. Waiting for activation...")
		if err = v.WaitForActivation(ctx); err != nil {
			log.WithError(err).Warn("Could not wait for validator activation")
		}
	}
}

func performRoles(slotCtx context.Context, allRoles map[[48]byte][]iface.ValidatorRole, v iface.Validator, slot primitives.Slot, wg *sync.WaitGroup, span trace.Span) {
	for pubKey, roles := range allRoles {
		wg.Add(len(roles))
		for _, role := range roles {
			go func(role iface.ValidatorRole, pubKey [fieldparams.BLSPubkeyLength]byte) {
				defer wg.Done()
				switch role {
				case iface.RoleAttester:
					v.SubmitAttestation(slotCtx, slot, pubKey)
				case iface.RoleProposer:
					v.ProposeBlock(slotCtx, slot, pubKey)
				case iface.RoleAggregator:
					v.SubmitAggregateAndProof(slotCtx, slot, pubKey)
				case iface.RoleSyncCommittee:
					v.SubmitSyncCommitteeMessage(slotCtx, slot, pubKey)
				case iface.RoleSyncCommitteeAggregator:
					v.SubmitSignedContributionAndProof(slotCtx, slot, pubKey)
				case iface.RoleUnknown:
					log.WithField("pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:]))).Trace("No active roles, doing nothing")
				default:
					log.Warnf("Unhandled role %v", role)
				}
			}(role, pubKey)
		}
	}

	// Wait for all processes to complete, then report span complete.
	go func() {
		wg.Wait()
		defer span.End()
		defer func() {
			if err := recover(); err != nil { // catch any panic in logging
				log.WithField("error", err).
					Error("Panic occurred when logging validator report. This" +
						" should never happen! Please file a report at github.com/prysmaticlabs/prysm/issues/new")
			}
		}()
		// Log performance in the previous slot
		v.LogSubmittedAtts(slot)
		v.LogSubmittedSyncCommitteeMessages()
		if err := v.LogValidatorGainsAndLosses(slotCtx, slot); err != nil {
			log.WithError(err).Error("Could not report validator's rewards/penalties")
		}
	}()
}

func isConnectionError(err error) bool {
	return err != nil && errors.Is(err, client.ErrConnectionIssue)
}

func handleAssignmentError(err error, slot primitives.Slot) {
	if errors.Is(err, ErrValidatorsAllExited) {
		log.Warn(ErrValidatorsAllExited)
	} else if errCode, ok := status.FromError(err); ok && errCode.Code() == codes.NotFound {
		log.WithField(
			"epoch", slot/params.BeaconConfig().SlotsPerEpoch,
		).Warn("Validator not yet assigned to epoch")
	} else {
		log.WithError(err).Error("Failed to update assignments")
	}
}

func runHealthCheckRoutine(ctx context.Context, v iface.Validator) {
	log.Info("Starting health check routine for beacon node apis")
	healthCheckTicker := time.NewTicker(time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	tracker := v.HealthTracker()
	go func() {
		// trigger the healthcheck immediately the first time
		for ; true; <-healthCheckTicker.C {
			if ctx.Err() != nil {
				log.WithError(ctx.Err()).Error("Context cancelled")
				return
			}
			isHealthy := tracker.CheckHealth(ctx)
			if !isHealthy && features.Get().EnableBeaconRESTApi {
				v.ChangeHost()
				if !tracker.CheckHealth(ctx) {
					continue // Skip to the next ticker
				}
			}

			// in case of node returning healthy but event stream died
			if isHealthy && !v.EventStreamIsRunning() {
				log.Info("Event stream reconnecting...")
				go v.StartEventStream(ctx, event.DefaultEventTopics)
			}
		}
	}()
}
