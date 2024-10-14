package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/client/event"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	prysmTrace "github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Time to wait before trying to reconnect with beacon node.
var backOffPeriod = 10 * time.Second

// Run the main validator routine. This routine exits if the context is
// canceled.
//
// Order of operations:
// 1 - Initialize validator data
// 2 - Wait for validator activation
// 3 - Wait for the next slot start
// 4 - Update assignments
// 5 - Determine role at current slot
// 6 - Perform assigned role, if any
func run(ctx context.Context, v iface.Validator) {
	cleanup := v.Done
	defer cleanup()

	headSlot, err := initializeValidatorAndGetHeadSlot(ctx, v)
	if err != nil {
		return // Exit if context is canceled.
	}
	if err := v.UpdateDuties(ctx, headSlot); err != nil {
		handleAssignmentError(err, headSlot)
	}
	eventsChan := make(chan *event.Event, 1)
	healthTracker := v.HealthTracker()
	runHealthCheckRoutine(ctx, v, eventsChan)

	accountsChangedChan := make(chan [][fieldparams.BLSPubkeyLength]byte, 1)
	km, err := v.Keymanager()
	if err != nil {
		log.WithError(err).Fatal("Could not get keymanager")
	}
	sub := km.SubscribeAccountChanges(accountsChangedChan)
	// check if proposer settings is still nil
	// Set properties on the beacon node like the fee recipient for validators that are being used & active.
	if v.ProposerSettings() == nil {
		log.Warn("Validator client started without proposer settings such as fee recipient" +
			" and will continue to use settings provided in the beacon node.")
	}
	if err := v.PushProposerSettings(ctx, km, headSlot, true); err != nil {
		log.WithError(err).Fatal("Failed to update proposer settings")
	}
	for {
		ctx, span := prysmTrace.StartSpan(ctx, "validator.processSlot")
		select {
		case <-ctx.Done():
			log.Info("Context canceled, stopping validator")
			span.End()
			sub.Unsubscribe()
			close(accountsChangedChan)
			return // Exit if context is canceled.
		case slot := <-v.NextSlot():
			if !healthTracker.IsHealthy() {
				continue
			}
			span.SetAttributes(prysmTrace.Int64Attribute("slot", int64(slot))) // lint:ignore uintcast -- This conversion is OK for tracing.

			deadline := v.SlotDeadline(slot)
			slotCtx, cancel := context.WithDeadline(ctx, deadline)
			log := log.WithField("slot", slot)
			log.WithField("deadline", deadline).Debug("Set deadline for proposals and attestations")

			// Keep trying to update assignments if they are nil or if we are past an
			// epoch transition in the beacon node's state.
			if err := v.UpdateDuties(ctx, slot); err != nil {
				handleAssignmentError(err, slot)
				cancel()
				span.End()
				continue
			}

			// call push proposer settings often to account for the following edge cases:
			// proposer is activated at the start of epoch and tries to propose immediately
			// account has changed in the middle of an epoch
			if err := v.PushProposerSettings(ctx, km, slot, false); err != nil {
				log.WithError(err).Warn("Failed to update proposer settings")
			}

			// Start fetching domain data for the next epoch.
			if slots.IsEpochEnd(slot) {
				go v.UpdateDomainDataCaches(ctx, slot+1)
			}

			var wg sync.WaitGroup

			allRoles, err := v.RolesAt(ctx, slot)
			if err != nil {
				log.WithError(err).Error("Could not get validator roles")
				cancel()
				span.End()
				continue
			}
			performRoles(slotCtx, allRoles, v, slot, &wg, span)
		case isHealthyAgain := <-healthTracker.HealthUpdates():
			if isHealthyAgain {
				headSlot, err = initializeValidatorAndGetHeadSlot(ctx, v)
				if err != nil {
					log.WithError(err).Error("Failed to re initialize validator and get head slot")
					continue
				}
				if err := v.UpdateDuties(ctx, headSlot); err != nil {
					handleAssignmentError(err, headSlot)
					continue
				}
			}
		case e := <-eventsChan:
			v.ProcessEvent(e)
		case currentKeys := <-accountsChangedChan: // should be less of a priority than next slot
			onAccountsChanged(ctx, v, currentKeys, accountsChangedChan)
		}
	}
}

func onAccountsChanged(ctx context.Context, v iface.Validator, current [][48]byte, ac chan [][fieldparams.BLSPubkeyLength]byte) {
	anyActive, err := v.HandleKeyReload(ctx, current)
	if err != nil {
		log.WithError(err).Error("Could not properly handle reloaded keys")
	}
	if !anyActive {
		log.Warn("No active keys found. Waiting for activation...")
		err := v.WaitForActivation(ctx, ac)
		if err != nil {
			log.WithError(err).Warn("Could not wait for validator activation")
		}
	}
}

func initializeValidatorAndGetHeadSlot(ctx context.Context, v iface.Validator) (primitives.Slot, error) {
	ctx, span := prysmTrace.StartSpan(ctx, "validator.initializeValidatorAndGetHeadSlot")
	defer span.End()

	ticker := time.NewTicker(backOffPeriod)
	defer ticker.Stop()

	firstTime := true

	var (
		headSlot primitives.Slot
		err      error
	)

	for {
		if !firstTime {
			if ctx.Err() != nil {
				log.Info("Context canceled, stopping validator")
				return headSlot, errors.New("context canceled")
			}
			<-ticker.C
		}

		firstTime = false

		if err := v.WaitForChainStart(ctx); err != nil {
			if isConnectionError(err) {
				log.WithError(err).Warn("Could not determine if beacon chain started")
				continue
			}

			log.WithError(err).Fatal("Could not determine if beacon chain started")
		}

		if err := v.WaitForKeymanagerInitialization(ctx); err != nil {
			// log.Fatal will prevent defer from being called
			v.Done()
			log.WithError(err).Fatal("Wallet is not ready")
		}

		if err := v.WaitForSync(ctx); err != nil {
			if isConnectionError(err) {
				log.WithError(err).Warn("Could not determine if beacon chain started")
				continue
			}

			log.WithError(err).Fatal("Could not determine if beacon node synced")
		}

		if err := v.WaitForActivation(ctx, nil /* accountsChangedChan */); err != nil {
			log.WithError(err).Fatal("Could not wait for validator activation")
		}

		headSlot, err = v.CanonicalHeadSlot(ctx)
		if isConnectionError(err) {
			log.WithError(err).Warn("Could not get current canonical head slot")
			continue
		}

		if err != nil {
			log.WithError(err).Fatal("Could not get current canonical head slot")
		}

		if err := v.CheckDoppelGanger(ctx); err != nil {
			if isConnectionError(err) {
				log.WithError(err).Warn("Could not wait for checking doppelganger")
				continue
			}

			log.WithError(err).Fatal("Could not succeed with doppelganger check")
		}
		break
	}
	return headSlot, nil
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
				case iface.RolePayloadTimelinessCommittee:
					v.SubmitPayloadAttestationMessage(slotCtx, slot, pubKey)
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

func runHealthCheckRoutine(ctx context.Context, v iface.Validator, eventsChan chan<- *event.Event) {
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

				km, err := v.Keymanager()
				if err != nil {
					log.WithError(err).Error("Could not get keymanager")
					return
				}
				slot, err := v.CanonicalHeadSlot(ctx)
				if err != nil {
					log.WithError(err).Error("Could not get canonical head slot")
					return
				}
				if err := v.PushProposerSettings(ctx, km, slot, true); err != nil {
					log.WithError(err).Warn("Failed to update proposer settings")
				}
			}

			// in case of node returning healthy but event stream died
			if isHealthy && !v.EventStreamIsRunning() {
				log.Info("Event stream reconnecting...")
				go v.StartEventStream(ctx, event.DefaultEventTopics, eventsChan)
			}
		}
	}()
}
