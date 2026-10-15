package iface

import "github.com/OffchainLabs/prysm/v7/consensus-types/primitives"

// InclusionListDutiesResponse holds the inclusion list committee duties of an epoch, from the beacon API.
type InclusionListDutiesResponse struct {
	DependentRoot       []byte
	ExecutionOptimistic bool
	Duties              []*InclusionListDuty
}

// InclusionListDuty is one inclusion list committee assignment.
type InclusionListDuty struct {
	Pubkey         []byte
	ValidatorIndex primitives.ValidatorIndex
	Slot           primitives.Slot
}

// GetDependentRoot returns the dependent root, or nil for a nil response.
func (r *InclusionListDutiesResponse) GetDependentRoot() []byte {
	if r == nil {
		return nil
	}
	return r.DependentRoot
}
