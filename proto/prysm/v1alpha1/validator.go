package eth

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

// ValidatorDutiesContainer is a wrapper that can be both used for the gRPC DutiesResponse and Rest API response structs for attestation, proposer, and sync duties.
type ValidatorDutiesContainer struct {
	PrevDependentRoot  []byte
	CurrDependentRoot  []byte
	CurrentEpochDuties []*ValidatorDuty
	NextEpochDuties    []*ValidatorDuty
}

// ValidatorDuty is all the information needed to execute validator duties
type ValidatorDuty struct {
	CommitteeLength         uint64
	CommitteeIndex          primitives.CommitteeIndex
	CommitteesAtSlot        uint64
	ValidatorCommitteeIndex uint64
	AttesterSlot            primitives.Slot
	ProposerSlots           []primitives.Slot
	PublicKey               []byte
	Status                  ValidatorStatus
	ValidatorIndex          primitives.ValidatorIndex
	IsSyncCommittee         bool
}
