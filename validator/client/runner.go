package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/api/client"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	prysmTrace "github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Time to wait before trying to reconnect with beacon node.
var backOffPeriod = 10 * time.Second

// runner encapsulates the main validator routine.
type runner struct {
	validator     iface.Validator
	healthMonitor *healthMonitor
}

// newRunner creates a new runner instance and performs all necessary initialization.
// This function can return an error if initialization fails.
//
// Order of operations:
// 1 - Initialize validator data
// 2 - Wait for validator activation
func newRunner(ctx context.Context, v iface.Validator, monitor *healthMonitor) (*runner, error) {
	// Initialize validator and get head slot
	err := initialize(ctx, v)
	if err != nil {
		v.Done()
		return nil, err
	}
	currentSlot := slots.CurrentSlot(v.GenesisTime()) // set in v.WaitForChainStart
	// Prepare initial duties update
	ss, err := slots.EpochStart(slots.ToEpoch(currentSlot + 1))
	if err != nil {
		log.WithError(err).Error("Failed to get epoch start")
		ss = currentSlot
	}
	startDeadline := v.SlotDeadline(ss + params.BeaconConfig().SlotsPerEpoch - 1)
	startCtx, startCancel := context.WithDeadline(ctx, startDeadline)
	if err := v.UpdateDuties(startCtx); err != nil {
		// Don't return error here, just log it
		handleAssignmentError(err, currentSlot)
	}
	startCancel()

	// check if proposer settings is still nil
	// Set properties on the beacon node like the fee recipient for validators that are being used & active.
	if v.ProposerSettings() == nil {
		log.Warn("Validator client started without proposer settings such as fee recipient" +
			" and will continue to use settings provided in the beacon node.")
	}
	if err := v.PushProposerSettings(ctx, currentSlot, true); err != nil {
		v.Done()
		return nil, errors.Wrap(err, "failed to update proposer settings")
	}
	return &runner{
		validator:     v,
		healthMonitor: monitor,
	}, nil
}

// run executes the main validator routine. This routine exits if the context is
// canceled. It returns a channel that will be closed when the routine exits.
//
// Order of operations:
// 1 - Wait for the next slot start
// 2 - Update assignments if needed
// 3 - Determine role at current slot
// 4 - Perform assigned role, if any
func (r *runner) run(ctx context.Context) {
	v := r.validator
	cleanup := v.Done
	defer cleanup()
	v.SetTicker()
	for {
		select {
		case <-ctx.Done():
			log.Info("Context canceled, stopping validator")
			//nolint:govet
			return // Exit if context is canceled.
		case slot := <-v.NextSlot():
			if !r.healthMonitor.IsHealthy() {
				log.Warn("Beacon node unhealthy, stopping runner")
				return
			}

			deadline := v.SlotDeadline(slot)
			slotCtx, cancel := context.WithDeadline(ctx, deadline) //nolint:govet

			var span trace.Span
			slotCtx, span = prysmTrace.StartSpan(slotCtx, "validator.processSlot")
			span.SetAttributes(prysmTrace.Int64Attribute("slot", int64(slot))) // lint:ignore uintcast -- This conversion is OK for tracing.

			log := log.WithField("slot", slot)
			log.WithField("deadline", deadline).Debug("Set deadline for proposals and attestations")

			// Keep trying to update assignments if they are nil or if we are past an
			// epoch transition in the beacon node's state.
			if slots.IsEpochStart(slot) {
				deadline = v.SlotDeadline(slot + params.BeaconConfig().SlotsPerEpoch - 1)
				dutiesCtx, dutiesCancel := context.WithDeadline(ctx, deadline)
				if err := v.UpdateDuties(dutiesCtx); err != nil {
					handleAssignmentError(err, slot)
					dutiesCancel()
					span.End()
					cancel()
					continue
				}
				dutiesCancel()
			}

			// call push proposer settings often to account for the following edge cases:
			// proposer is activated at the start of epoch and tries to propose immediately
			// account has changed in the middle of an epoch
			if err := v.PushProposerSettings(slotCtx, slot, false); err != nil {
				log.WithError(err).Warn("Failed to update proposer settings")
			}

			// Start fetching domain data for the next epoch.
			if slots.IsEpochEnd(slot) {
				domainCtx, _ := context.WithDeadline(ctx, deadline) //nolint:govet
				go v.UpdateDomainDataCaches(domainCtx, slot+1)
			}

			var wg sync.WaitGroup

			allRoles, err := v.RolesAt(slotCtx, slot)
			if err != nil {
				log.WithError(err).Error("Could not get validator roles")
				span.End()
				cancel()
				continue
			}
			// performRoles calls span.End()
			rolesCtx, _ := context.WithDeadline(ctx, deadline) //nolint:govet
			performRoles(rolesCtx, allRoles, v, slot, &wg, span)
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
		err := v.WaitForActivation(ctx)
		if err != nil {
			log.WithError(err).Warn("Could not wait for validator activation")
		} else {
			log.Debug("Resetting slot ticker after waiting for validator activation.")
			v.SetTicker()
		}
	}
}

func initialize(ctx context.Context, v iface.Validator) error {
	ctx, span := prysmTrace.StartSpan(ctx, "validator.initialize")
	defer span.End()

	ticker := time.NewTicker(backOffPeriod)
	defer ticker.Stop()

	firstTime := true

	for {
		if !firstTime {
			if ctx.Err() != nil {
				log.Info("Context canceled, stopping validator")
				return errors.New("context canceled")
			}
			<-ticker.C
		}

		firstTime = false

		if err := v.WaitForChainStart(ctx); err != nil {
			if isConnectionError(err) {
				log.WithError(err).Warn("Could not determine if beacon chain started")
				continue
			}

			return errors.Wrap(err, "could not determine if beacon chain started")
		}

		if err := v.WaitForKeymanagerInitialization(ctx); err != nil {
			return errors.Wrap(err, "Wallet is not ready")
		}

		if err := v.WaitForSync(ctx); err != nil {
			if isConnectionError(err) {
				log.WithError(err).Warn("Could not determine if beacon chain started")
				continue
			}

			return errors.Wrap(err, "could not determine if beacon node synced")
		}

		if err := v.WaitForActivation(ctx); err != nil {
			return errors.Wrap(err, "could not wait for validator activation")
		}

		if err := v.CheckDoppelGanger(ctx); err != nil {
			if isConnectionError(err) {
				log.WithError(err).Warn("Could not wait for checking doppelganger")
				continue
			}

			return errors.Wrap(err, "could not succeed with doppelganger check")
		}
		break
	}

	return nil
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
