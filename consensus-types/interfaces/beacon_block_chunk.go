package interfaces

import (
	field_params "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

// ReadOnlyBeaconBlockChunk is an interface describing the method set of
// a signed beacon block chunk
type ReadOnlyBeaconBlockChunk interface {
	IsNil() bool
	Version() int
	Slot() primitives.Slot
	ProposerIndex() primitives.ValidatorIndex
	ParentRoot() [field_params.RootLength]byte
	Commitments() [][]byte
	Signature() [field_params.BLSSignatureLength]byte
	Header() *ethpb.BeaconBlockChunkHeader
	HeaderRoot() [field_params.RootLength]byte
	Data() [][]byte
	Coefficients() [][]byte
}

type BeaconBlockChunk interface {
	ReadOnlyBeaconBlockChunk
	SetParentRoot([]byte)
	SetProposerIndex(idx primitives.ValidatorIndex)
	SetSlot(slot primitives.Slot)
	SetSignature(sig [96]byte)
	SetVersion(version int)
	SetCommitments(commitments [][]byte)
	SetData(data [][]byte)
	SetCoefficients(coefficients [][]byte)
}
