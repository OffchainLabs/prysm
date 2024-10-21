package light_client

import (
	"fmt"

	consensustypes "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"google.golang.org/protobuf/proto"
)

func NewWrappedOptimisticUpdate(m proto.Message) (interfaces.LightClientOptimisticUpdate, error) {
	if m == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	switch t := m.(type) {
	case *pb.LightClientOptimisticUpdateAltair:
		return NewWrappedOptimisticUpdateAltair(t)
	case *pb.LightClientOptimisticUpdateCapella:
		return NewWrappedOptimisticUpdateCapella(t)
	case *pb.LightClientOptimisticUpdateDeneb:
		return NewWrappedOptimisticUpdateDeneb(t)
	default:
		return nil, fmt.Errorf("cannot construct light client optimistic update from type %T", t)
	}
}

func NewOptimisticUpdateFromUpdate(update interfaces.LightClientUpdate) (interfaces.LightClientOptimisticUpdate, error) {
	switch t := update.(type) {
	case *updateAltair:
		return &optimisticUpdateAltair{
			p: &pb.LightClientOptimisticUpdateAltair{
				AttestedHeader: t.p.AttestedHeader,
				SyncAggregate:  t.p.SyncAggregate,
				SignatureSlot:  t.p.SignatureSlot,
			},
			attestedHeader: t.attestedHeader,
		}, nil
	case *updateCapella:
		return &optimisticUpdateCapella{
			p: &pb.LightClientOptimisticUpdateCapella{
				AttestedHeader: t.p.AttestedHeader,
				SyncAggregate:  t.p.SyncAggregate,
				SignatureSlot:  t.p.SignatureSlot,
			},
			attestedHeader: t.attestedHeader,
		}, nil
	case *updateDeneb:
		return &optimisticUpdateDeneb{
			p: &pb.LightClientOptimisticUpdateDeneb{
				AttestedHeader: t.p.AttestedHeader,
				SyncAggregate:  t.p.SyncAggregate,
				SignatureSlot:  t.p.SignatureSlot,
			},
			attestedHeader: t.attestedHeader,
		}, nil
	case *updateElectra:
		return &optimisticUpdateDeneb{
			p: &pb.LightClientOptimisticUpdateDeneb{
				AttestedHeader: t.p.AttestedHeader,
				SyncAggregate:  t.p.SyncAggregate,
				SignatureSlot:  t.p.SignatureSlot,
			},
			attestedHeader: t.attestedHeader,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported type %T", t)
	}
}

type optimisticUpdateAltair struct {
	p              *pb.LightClientOptimisticUpdateAltair
	attestedHeader interfaces.LightClientHeader
}

var _ interfaces.LightClientOptimisticUpdate = &optimisticUpdateAltair{}

func NewWrappedOptimisticUpdateAltair(p *pb.LightClientOptimisticUpdateAltair) (interfaces.LightClientOptimisticUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderAltair(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	return &optimisticUpdateAltair{
		p:              p,
		attestedHeader: attestedHeader,
	}, nil
}

func (u *optimisticUpdateAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *optimisticUpdateAltair) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *optimisticUpdateAltair) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *optimisticUpdateAltair) Proto() proto.Message {
	return u.p
}

func (u *optimisticUpdateAltair) Version() int {
	return version.Altair
}

func (u *optimisticUpdateAltair) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *optimisticUpdateAltair) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *optimisticUpdateAltair) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

type optimisticUpdateCapella struct {
	p              *pb.LightClientOptimisticUpdateCapella
	attestedHeader interfaces.LightClientHeader
}

var _ interfaces.LightClientOptimisticUpdate = &optimisticUpdateCapella{}

func NewWrappedOptimisticUpdateCapella(p *pb.LightClientOptimisticUpdateCapella) (interfaces.LightClientOptimisticUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderCapella(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	return &optimisticUpdateCapella{
		p:              p,
		attestedHeader: attestedHeader,
	}, nil
}

func (u *optimisticUpdateCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *optimisticUpdateCapella) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *optimisticUpdateCapella) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *optimisticUpdateCapella) Proto() proto.Message {
	return u.p
}

func (u *optimisticUpdateCapella) Version() int {
	return version.Capella
}

func (u *optimisticUpdateCapella) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *optimisticUpdateCapella) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *optimisticUpdateCapella) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

type optimisticUpdateDeneb struct {
	p              *pb.LightClientOptimisticUpdateDeneb
	attestedHeader interfaces.LightClientHeader
}

var _ interfaces.LightClientOptimisticUpdate = &optimisticUpdateDeneb{}

func NewWrappedOptimisticUpdateDeneb(p *pb.LightClientOptimisticUpdateDeneb) (interfaces.LightClientOptimisticUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderDeneb(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	return &optimisticUpdateDeneb{
		p:              p,
		attestedHeader: attestedHeader,
	}, nil
}

func (u *optimisticUpdateDeneb) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *optimisticUpdateDeneb) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *optimisticUpdateDeneb) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *optimisticUpdateDeneb) Proto() proto.Message {
	return u.p
}

func (u *optimisticUpdateDeneb) Version() int {
	return version.Deneb
}

func (u *optimisticUpdateDeneb) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *optimisticUpdateDeneb) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *optimisticUpdateDeneb) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}
