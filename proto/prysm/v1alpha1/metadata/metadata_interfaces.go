package metadata

import (
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/go-bitfield"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// Metadata returns the interface of a p2p metadata type.
type Metadata interface {
	SequenceNumber() uint64
	AttnetsBitfield() bitfield.Bitvector64
	SyncnetsBitfield() bitfield.Bitvector4
	CustodySubnetCount() uint64
	InnerObject() interface{}
	IsNil() bool
	Copy() Metadata
	ssz.Marshaler
	ssz.Unmarshaler
	MetadataObjV0() *pb.MetaDataV0
	MetadataObjV1() *pb.MetaDataV1
	MetadataObjV2() *pb.MetaDataV2
	Version() int
}
