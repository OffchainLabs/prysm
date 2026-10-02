package light_client

import (
	"fmt"

	consensustypes "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"google.golang.org/protobuf/proto"
)

type optimisticUpdateGloas struct {
	p              *pb.LightClientOptimisticUpdateGloas
	attestedHeader interfaces.LightClientHeader
}

// NewEmptyOptimisticUpdateGloas creates an empty wrapper for SSZ unmarshalling.
func NewEmptyOptimisticUpdateGloas() interfaces.LightClientOptimisticUpdate {
	return &optimisticUpdateGloas{}
}

func (u *optimisticUpdateGloas) IsNil() bool {
	return u == nil || u.p == nil
}

var _ interfaces.LightClientOptimisticUpdate = &optimisticUpdateGloas{}

func NewWrappedOptimisticUpdateGloas(p *pb.LightClientOptimisticUpdateGloas) (interfaces.LightClientOptimisticUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderGloas(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	return &optimisticUpdateGloas{
		p:              p,
		attestedHeader: attestedHeader,
	}, nil
}

func (u *optimisticUpdateGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *optimisticUpdateGloas) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *optimisticUpdateGloas) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *optimisticUpdateGloas) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientOptimisticUpdateGloas{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedOptimisticUpdateGloas(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*optimisticUpdateGloas)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *optimisticUpdateGloas) Proto() proto.Message {
	return u.p
}

func (u *optimisticUpdateGloas) Version() int {
	return version.Gloas
}

func (u *optimisticUpdateGloas) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *optimisticUpdateGloas) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *optimisticUpdateGloas) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}
