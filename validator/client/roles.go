package client

// validatorRole defines the validator role.
type validatorRole int8

const (
	// RoleUnknown means that the role of the validator cannot be determined.
	RoleUnknown validatorRole = iota
	// RoleAttester means that the validator should submit an attestation.
	RoleAttester
	// RoleProposer means that the validator should propose a block.
	RoleProposer
	// RoleAggregator means that the validator should submit an aggregation and proof.
	RoleAggregator
	// RoleSyncCommittee means that the validator should submit a sync committee message.
	RoleSyncCommittee
	// RoleSyncCommitteeAggregator means the validator should aggregate sync committee messages and submit a sync committee contribution.
	RoleSyncCommitteeAggregator
	// RolePTCMember means the validator should submit a payload attestation.
	RolePTCMember
)
