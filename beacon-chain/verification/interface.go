package verification

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	payloadattestation "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attestation"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// BlobVerifier defines the methods implemented by the ROBlobVerifier.
// It is mainly intended to make mocks and tests more straightforward, and to deal
// with the awkwardness of mocking a concrete type that returns a concrete type
// in tests outside of this package.
type BlobVerifier interface {
	VerifiedROBlob() (blocks.VerifiedROBlob, error)
	BlobIndexInBounds() (err error)
	NotFromFutureSlot() (err error)
	SlotAboveFinalized() (err error)
	ValidProposerSignature(ctx context.Context) (err error)
	SidecarParentSeen(parentSeen func([32]byte) bool) (err error)
	SidecarParentValid(badParent func([32]byte) bool) (err error)
	SidecarParentSlotLower() (err error)
	SidecarDescendsFromFinalized() (err error)
	SidecarInclusionProven() (err error)
	SidecarKzgProofVerified() (err error)
	SidecarProposerExpected(ctx context.Context) (err error)
	SatisfyRequirement(Requirement)
}

// NewBlobVerifier is a function signature that can be used by code that needs to be
// able to mock Initializer.NewBlobVerifier without complex setup.
type NewBlobVerifier func(b blocks.ROBlob, reqs []Requirement) BlobVerifier

// DataColumnsVerifier defines the methods implemented by the RODataColumnVerifier.
// It serves a very similar purpose as the blob verifier interface for data columns.
type DataColumnsVerifier interface {
	VerifiedRODataColumns() ([]blocks.VerifiedRODataColumn, error)
	SatisfyRequirement(Requirement)

	ValidFields() error
	CorrectSubnet(dataColumnSidecarSubTopic string, expectedTopics []string) error
	NotFromFutureSlot() error
	SlotAboveFinalized() error
	ValidProposerSignature(ctx context.Context) error
	SidecarParentSeen(parentSeen func([fieldparams.RootLength]byte) bool) error
	SidecarParentValid(badParent func([fieldparams.RootLength]byte) bool) error
	SidecarParentSlotLower() error
	SidecarDescendsFromFinalized() error
	SidecarInclusionProven() error
	SidecarKzgProofVerified() error
	SidecarProposerExpected(ctx context.Context) error
}

// NewDataColumnsVerifier is a function signature that can be used to mock a setup where a
// column verifier can be easily initialized.
type NewDataColumnsVerifier func(dataColumns []blocks.RODataColumn, reqs []Requirement) DataColumnsVerifier

type GloasDataColumnVerifier interface {
	VerifiedRODataColumn() (blocks.VerifiedRODataColumn, error)
	SatisfyRequirement(Requirement)
	VerifyDataColumnSidecarSlotMatchesBlockGloas() error
	VerifyDataColumnSidecarGloas() error
	CorrectSubnet(dataColumnSidecarSubTopic string, expectedTopics []string) error
	VerifyDataColumnSidecarKzgProofsGloas() error
}

// PayloadAttestationMsgVerifier defines the methods implemented by the ROPayloadAttestation.
type PayloadAttestationMsgVerifier interface {
	VerifyCurrentSlot() error
	VerifyBlockRootSeen(blockRootSeen func([32]byte) bool) error
	VerifyBlockRootValid(func([32]byte) bool) error
	VerifyValidatorInPTC(context.Context, state.ReadOnlyBeaconState) error
	VerifySignature(state.ReadOnlyBeaconState) error
	VerifiedPayloadAttestation() (payloadattestation.VerifiedROMessage, error)
	SatisfyRequirement(Requirement)
}

// NewPayloadAttestationMsgVerifier is a function signature that can be used by code that needs to be
// able to mock Initializer.NewPayloadAttestationMsgVerifier without complex setup.
type NewPayloadAttestationMsgVerifier func(pa payloadattestation.ROMessage, reqs []Requirement) PayloadAttestationMsgVerifier

// SignedProposerPreferencesVerifier defines the methods implemented by the signed proposer preferences verifier.
type SignedProposerPreferencesVerifier interface {
	VerifyNextEpoch(state.ReadOnlyBeaconState) error
	VerifyValidProposalSlot(state.ReadOnlyBeaconState) error
	VerifySignature(state.ReadOnlyBeaconState) error
	SatisfyRequirement(Requirement)
}

// NewSignedProposerPreferencesVerifier is a function signature that can be used by code that needs to be
// able to mock Initializer.NewSignedProposerPreferencesVerifier without complex setup.
type NewSignedProposerPreferencesVerifier func(p *ethpb.SignedProposerPreferences, reqs []Requirement) SignedProposerPreferencesVerifier
