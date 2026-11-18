package state

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type writeOnlyPQFields interface {
	SetAggregationCommitteeWindow([]*ethpb.AggregationCommittees) error
	RotateAggregationCommitteeWindow([]*ethpb.AggregationCommittees) error
}

type readOnlyPQFields interface {
	AggregationCommitteeWindow() ([]*ethpb.AggregationCommittees, error)
	AggregationCommittee(slot primitives.Slot, index primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error)
	VrfRootAtIndex(idx primitives.ValidatorIndex) ([32]byte, error)
}
