package slots

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	mathutil "github.com/OffchainLabs/prysm/v7/math"
	"github.com/pkg/errors"
)

// Spec: R, PDF §3.1.
func SlotsPerRoundAt(slot primitives.Slot) uint64 {
	cfg := params.BeaconConfig()
	spr := uint64(cfg.SlotsPerEpoch)
	for _, e := range cfg.RoundSchedule {
		if slot < e.Slot {
			break
		}
		spr = e.SlotsPerRound
	}
	return spr
}

func RoundsPerEpochAt(slot primitives.Slot) uint64 {
	return uint64(params.BeaconConfig().SlotsPerEpoch) / SlotsPerRoundAt(slot)
}

// Spec: round(s) = ⌊s/R⌋, PDF §3.1.
func ToRound(slot primitives.Slot) primitives.Round {
	cfg := params.BeaconConfig()
	eraStart := primitives.Slot(0)
	startRound := primitives.Round(0)
	spr := uint64(cfg.SlotsPerEpoch)
	for _, e := range cfg.RoundSchedule {
		if slot < e.Slot {
			break
		}
		eraStart, startRound, spr = e.Slot, e.StartRound, e.SlotsPerRound
	}
	return startRound + primitives.Round(uint64(slot-eraStart)/spr)
}

// Spec: the opening slot rR of round r, PDF §3.1.
func RoundStart(round primitives.Round) (primitives.Slot, error) {
	cfg := params.BeaconConfig()
	eraStart := primitives.Slot(0)
	startRound := primitives.Round(0)
	spr := uint64(cfg.SlotsPerEpoch)
	for _, e := range cfg.RoundSchedule {
		if round < e.StartRound {
			break
		}
		eraStart, startRound, spr = e.Slot, e.StartRound, e.SlotsPerRound
	}
	offset, err := mathutil.Mul64(uint64(round-startRound), spr)
	if err != nil {
		return 0, errors.Wrap(errOverflow, "round start")
	}
	total, err := mathutil.Add64(uint64(eraStart), offset)
	if err != nil {
		return 0, errors.Wrap(errOverflow, "round start")
	}
	return primitives.Slot(total), nil
}

func EpochAtRound(round primitives.Round) (primitives.Epoch, error) {
	s, err := RoundStart(round)
	if err != nil {
		return 0, err
	}
	return ToEpoch(s), nil
}
