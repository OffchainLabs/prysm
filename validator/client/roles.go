package client

// validatorRole defines the validator role.
type validatorRole int8

const (
	// roleUnknown means that the role of the validator cannot be determined.
	roleUnknown validatorRole = iota
	// roleAttester means that the validator should submit an attestation.
	roleAttester
	// roleProposer means that the validator should propose a block.
	roleProposer
	// roleAggregator means that the validator should submit an aggregation and proof.
	roleAggregator
	// roleSyncCommittee means that the validator should submit a sync committee message.
	roleSyncCommittee
	// roleSyncCommitteeAggregator means the validator should aggregate sync committee messages and submit a sync committee contribution.
	roleSyncCommitteeAggregator
	// rolePTCMember means the validator should submit a payload attestation.
	rolePTCMember
)
