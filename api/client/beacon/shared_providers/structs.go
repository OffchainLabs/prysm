package shared_providers

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

type AttesterDuty struct {
	CommitteeIndex          primitives.CommitteeIndex
	Slot                    primitives.Slot
	CommitteeLength         uint64
	ValidatorCommitteeIndex uint64
	CommitteesAtSlot        uint64
}

type ValidatorForDuty struct {
	Pubkey []byte
	Index  primitives.ValidatorIndex
	Status ethpb.ValidatorStatus
}
