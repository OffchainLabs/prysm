package attestations

import (
	"bytes"
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
	attaggregation "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// This prepares fork choice attestations by running batchForkChoiceAtts
// every prepareForkChoiceAttsPeriod.
func (s *Service) prepareForkChoiceAtts() {
	intervals := features.Get().AggregateIntervals
	slotDuration := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	// Adjust intervals for networks with a lower slot duration (Hive, e2e, etc)
	for {
		if intervals[len(intervals)-1] >= slotDuration {
			for i, offset := range intervals {
				intervals[i] = offset / 2
			}
		} else {
			break
		}
	}
	ticker := slots.NewSlotTickerWithIntervals(time.Unix(int64(s.genesisTime), 0), intervals[:])
	for {
		select {
		case slotInterval := <-ticker.C():
			t := time.Now()
			if err := s.batchForkChoiceAtts(s.ctx); err != nil {
				log.WithError(err).Error("Could not prepare attestations for fork choice")
			}
			switch slotInterval.Interval {
			case 0:
				duration := time.Since(t)
				log.WithField("duration", duration).Debug("Aggregated unaggregated attestations")
				batchForkChoiceAttsT1.Observe(float64(duration.Milliseconds()))
			case 1:
				batchForkChoiceAttsT2.Observe(float64(time.Since(t).Milliseconds()))
			}
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting routine")
			return
		}
	}
}

// This gets the attestations from the unaggregated, aggregated and block
// pool. Then finds the common data, aggregate and batch them for fork choice.
// The resulting attestations are saved in the fork choice pool.
func (s *Service) batchForkChoiceAtts(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "Operations.attestations.batchForkChoiceAtts")
	defer span.End()

	var atts []ethpb.Att
	if features.Get().EnableExperimentalAttestationPool {
		atts = append(s.cfg.Cache.GetAll(), s.cfg.Cache.ForkchoiceAttestations()...)
	} else {
		if err := s.cfg.Pool.AggregateUnaggregatedAttestations(ctx); err != nil {
			return err
		}
		atts = append(s.cfg.Pool.AggregatedAttestations(), s.cfg.Pool.BlockAttestations()...)
		atts = append(atts, s.cfg.Pool.ForkchoiceAttestations()...)
	}

	attsById := make(map[attestation.Id][]ethpb.Att, len(atts))

	// Consolidate attestations by aggregating them by similar data root.
	for _, att := range atts {
		seen, err := s.seen(att)
		if err != nil {
			return err
		}
		if seen {
			continue
		}

		id, err := attestation.NewId(att, attestation.Data)
		if err != nil {
			return errors.Wrap(err, "could not create attestation ID")
		}
		attsById[id] = append(attsById[id], att)
	}

	for _, atts := range attsById {
		if err := s.aggregateAndSaveForkChoiceAtts(atts); err != nil {
			return err
		}
	}

	if !features.Get().EnableExperimentalAttestationPool {
		for _, a := range s.cfg.Pool.BlockAttestations() {
			if err := s.cfg.Pool.DeleteBlockAttestation(a); err != nil {
				return err
			}
		}
	}

	return nil
}

// This aggregates a list of attestations using the aggregation algorithm defined in AggregateAttestations
// and saves the attestations for fork choice.
func (s *Service) aggregateAndSaveForkChoiceAtts(atts []ethpb.Att) error {
	clonedAtts := make([]ethpb.Att, len(atts))
	for i, a := range atts {
		clonedAtts[i] = a.Clone()
	}
	aggregatedAtts, err := attaggregation.Aggregate(clonedAtts)
	if err != nil {
		return err
	}

	return s.cfg.Pool.SaveForkchoiceAttestations(aggregatedAtts)
}

// This checks if the attestation has previously been aggregated for fork choice
// return true if yes, false if no.
func (s *Service) seen(att ethpb.Att) (bool, error) {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return false, errors.Wrap(err, "could not create attestation ID")
	}
	incomingBits := att.GetAggregationBits()
	savedBits, ok := s.forkChoiceProcessedAtts.Get(id)
	if ok {
		savedBitlist, ok := savedBits.(bitfield.Bitlist)
		if !ok {
			return false, errors.New("not a bit field")
		}
		if savedBitlist.Len() == incomingBits.Len() {
			// Returns true if the node has seen all the bits in the new bit field of the incoming attestation.
			if bytes.Equal(savedBitlist, incomingBits) {
				return true, nil
			}
			if c, err := savedBitlist.Contains(incomingBits); err != nil {
				return false, err
			} else if c {
				return true, nil
			}
			var err error
			// Update the bit fields by Or'ing them with the new ones.
			incomingBits, err = incomingBits.Or(savedBitlist)
			if err != nil {
				return false, err
			}
		}
	}

	s.forkChoiceProcessedAtts.Add(id, incomingBits)
	return false, nil
}
