package shared_providers

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

type AttesterDuty struct {
	committeeIndex          primitives.CommitteeIndex
	slot                    primitives.Slot
	committeeLength         uint64
	validatorCommitteeIndex uint64
	committeesAtSlot        uint64
}

type ValidatorForDuty struct {
	pubkey []byte
	index  primitives.ValidatorIndex
	status ethpb.ValidatorStatus
}
