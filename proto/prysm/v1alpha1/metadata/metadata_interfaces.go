package metadata

import (
	"github.com/OffchainLabs/go-bitfield"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	ssz "github.com/prysmaticlabs/fastssz"
)

// Metadata returns the interface of a p2p metadata type.
type Metadata interface {
	SequenceNumber() uint64
	AttnetsBitfield() bitfield.Bitvector64
	SyncnetsBitfield() bitfield.Bitvector4
	CustodyGroupCount() uint64
	InnerObject() any
	IsNil() bool
	Copy() Metadata
	ssz.Marshaler
	ssz.Unmarshaler
	MetadataObjV0() *pb.MetaDataV0
	MetadataObjV1() *pb.MetaDataV1
	MetadataObjV2() *pb.MetaDataV2
	Version() int
}
