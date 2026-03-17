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
	RequireBlockSeenGloas
	RequireSlotMatchesBlockGloas
	RequireValidFieldsGloas
	RequireSidecarKzgProofVerifiedGloas
	RequireNotSeenGloas

	// Payload attestation specific.
	RequireCurrentSlot
	RequireMessageNotSeen
	RequireValidatorInPTC
	RequireBlockRootSeen
	RequireBlockRootValid
	RequireSignatureValid

	// Execution payload envelope specific.
	RequireBuilderValid
	RequirePayloadHashValid
	RequireEnvelopeSlotAboveFinalized
	RequireEnvelopeSlotMatchesBlock
	RequireBuilderSignatureValid

	// Signed proposer preferences specific.
	RequireProposerPreferencesNextEpoch
	RequireProposerPreferencesProposalSlotValid
	RequireProposerPreferencesSignatureValid
)
