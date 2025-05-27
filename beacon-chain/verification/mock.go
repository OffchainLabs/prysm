package verification

import (
	"context"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
)

// MockBlobVerifier is a mock implementation of the BlobVerifier interface.
type MockBlobVerifier struct {
	ErrBlobIndexInBounds            error
	ErrSlotTooEarly                 error
	ErrSlotAboveFinalized           error
	ErrValidProposerSignature       error
	ErrSidecarParentSeen            error
	ErrSidecarParentValid           error
	ErrSidecarParentSlotLower       error
	ErrSidecarDescendsFromFinalized error
	ErrSidecarInclusionProven       error
	ErrSidecarKzgProofVerified      error
	ErrSidecarProposerExpected      error
	CbVerifiedROBlob                func() (blocks.VerifiedROBlob, error)
}

var _ BlobVerifier = &MockBlobVerifier{}

func (m *MockBlobVerifier) VerifiedROBlob() (blocks.VerifiedROBlob, error) {
	return m.CbVerifiedROBlob()
}

func (m *MockBlobVerifier) BlobIndexInBounds() (err error) {
	return m.ErrBlobIndexInBounds
}

func (m *MockBlobVerifier) NotFromFutureSlot() (err error) {
	return m.ErrSlotTooEarly
}

func (m *MockBlobVerifier) SlotAboveFinalized() (err error) {
	return m.ErrSlotAboveFinalized
}

func (m *MockBlobVerifier) ValidProposerSignature(_ context.Context) (err error) {
	return m.ErrValidProposerSignature
}

func (m *MockBlobVerifier) SidecarParentSeen(_ func([32]byte) bool) (err error) {
	return m.ErrSidecarParentSeen
}

func (m *MockBlobVerifier) SidecarParentValid(_ func([32]byte) bool) (err error) {
	return m.ErrSidecarParentValid
}

func (m *MockBlobVerifier) SidecarParentSlotLower() (err error) {
	return m.ErrSidecarParentSlotLower
}

func (m *MockBlobVerifier) SidecarDescendsFromFinalized() (err error) {
	return m.ErrSidecarDescendsFromFinalized
}

func (m *MockBlobVerifier) SidecarInclusionProven() (err error) {
	return m.ErrSidecarInclusionProven
}

func (m *MockBlobVerifier) SidecarKzgProofVerified() (err error) {
	return m.ErrSidecarKzgProofVerified
}

func (m *MockBlobVerifier) SidecarProposerExpected(_ context.Context) (err error) {
	return m.ErrSidecarProposerExpected
}

func (*MockBlobVerifier) SatisfyRequirement(_ Requirement) {}

// Data column sidecars
// --------------------

type MockDataColumnsVerifier struct {
	ErrValidFields                  error
	ErrCorrectSubnet                error
	ErrNotFromFutureSlot            error
	ErrSlotAboveFinalized           error
	ErrSidecarParentSeen            error
	ErrSidecarParentValid           error
	ErrValidProposerSignature       error
	ErrSidecarParentSlotLower       error
	ErrSidecarDescendsFromFinalized error
	ErrSidecarInclusionProven       error
	ErrSidecarKzgProofVerified      error
	ErrSidecarProposerExpected      error
}

var _ DataColumnsVerifier = &MockDataColumnsVerifier{}

func (m *MockDataColumnsVerifier) VerifiedRODataColumns() ([]blocks.VerifiedRODataColumn, error) {
	return []blocks.VerifiedRODataColumn{{}}, nil
}

func (m *MockDataColumnsVerifier) SatisfyRequirement(_ Requirement) {}

func (m *MockDataColumnsVerifier) ValidFields() error {
	return m.ErrValidFields
}

func (m *MockDataColumnsVerifier) CorrectSubnet(_ string, _ []string) error {
	return m.ErrCorrectSubnet
}

func (m *MockDataColumnsVerifier) NotFromFutureSlot() error {
	return m.ErrNotFromFutureSlot
}

func (m *MockDataColumnsVerifier) SlotAboveFinalized() error {
	return m.ErrSlotAboveFinalized
}

func (m *MockDataColumnsVerifier) ValidProposerSignature(_ context.Context) error {
	return m.ErrValidProposerSignature
}

func (m *MockDataColumnsVerifier) SidecarParentSeen(_ func([fieldparams.RootLength]byte) bool) error {
	return m.ErrSidecarParentSeen
}

func (m *MockDataColumnsVerifier) SidecarParentValid(_ func([fieldparams.RootLength]byte) bool) error {
	return m.ErrSidecarParentValid
}

func (m *MockDataColumnsVerifier) SidecarParentSlotLower() error {
	return m.ErrSidecarParentSlotLower
}

func (m *MockDataColumnsVerifier) SidecarDescendsFromFinalized() error {
	return m.ErrSidecarDescendsFromFinalized
}

func (m *MockDataColumnsVerifier) SidecarInclusionProven() error {
	return m.ErrSidecarInclusionProven
}

func (m *MockDataColumnsVerifier) SidecarKzgProofVerified() error {
	return m.ErrSidecarKzgProofVerified
}

func (m *MockDataColumnsVerifier) SidecarProposerExpected(_ context.Context) error {
	return m.ErrSidecarProposerExpected
}
