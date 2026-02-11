package verification

const (
	RequireBlobIndexInBounds Requirement = iota
	RequireNotFromFutureSlot
	RequireSlotAboveFinalized
	RequireValidProposerSignature
	RequireSidecarParentSeen
	RequireSidecarParentValid
	RequireSidecarParentSlotLower
	RequireSidecarDescendsFromFinalized
	RequireSidecarInclusionProven
	RequireSidecarKzgProofVerified
	RequireSidecarProposerExpected

	// Data columns specific.
	RequireValidFields
	RequireCorrectSubnet

	// Payload attestation specific.
	RequireCurrentSlot
	RequireMessageNotSeen
	RequireValidatorInPTC
	RequireBlockRootSeen
	RequireBlockRootValid
	RequireSignatureValid
)
