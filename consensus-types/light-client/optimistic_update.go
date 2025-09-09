package light_client

import (
	"fmt"

	consensustypes "github.com/OffchainLabs/prysm/v6/consensus-types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
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

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
type optimisticUpdateAltair struct {
	p              *pb.LightClientOptimisticUpdateAltair
	attestedHeader interfaces.LightClientHeader
}

// NewEmptyOptimisticUpdateAltair normally should never be called and NewOptimisticUpdateFromUpdate should be used instead.
// This function exists only for scenarios where an empty struct is required.
func NewEmptyOptimisticUpdateAltair() interfaces.LightClientOptimisticUpdate {
	return &optimisticUpdateAltair{}
}

func (u *optimisticUpdateAltair) IsNil() bool {
	return u == nil || u.p == nil
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

func (u *optimisticUpdateAltair) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientOptimisticUpdateAltair{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedOptimisticUpdateAltair(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*optimisticUpdateAltair)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *optimisticUpdateAltair) Proto() proto.Message {
	return u.p
}

func (u *optimisticUpdateAltair) Version() int {
	return slots.ToForkVersion(u.attestedHeader.Beacon().Slot)
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

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
type optimisticUpdateCapella struct {
	p              *pb.LightClientOptimisticUpdateCapella
	attestedHeader interfaces.LightClientHeader
}

// NewEmptyOptimisticUpdateCapella normally should never be called and NewOptimisticUpdateFromUpdate should be used instead.
// This function exists only for scenarios where an empty struct is required.
func NewEmptyOptimisticUpdateCapella() interfaces.LightClientOptimisticUpdate {
	return &optimisticUpdateCapella{}
}

func (u *optimisticUpdateCapella) IsNil() bool {
	return u == nil || u.p == nil
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

func (u *optimisticUpdateCapella) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientOptimisticUpdateCapella{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedOptimisticUpdateCapella(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*optimisticUpdateCapella)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *optimisticUpdateCapella) Proto() proto.Message {
	return u.p
}

func (u *optimisticUpdateCapella) Version() int {
	return slots.ToForkVersion(u.attestedHeader.Beacon().Slot)
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

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
type optimisticUpdateDeneb struct {
	p              *pb.LightClientOptimisticUpdateDeneb
	attestedHeader interfaces.LightClientHeader
}

// NewEmptyOptimisticUpdateDeneb normally should never be called and NewOptimisticUpdateFromUpdate should be used instead.
// This function exists only for scenarios where an empty struct is required.
func NewEmptyOptimisticUpdateDeneb() interfaces.LightClientOptimisticUpdate {
	return &optimisticUpdateDeneb{}
}

func (u *optimisticUpdateDeneb) IsNil() bool {
	return u == nil || u.p == nil
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

func (u *optimisticUpdateDeneb) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientOptimisticUpdateDeneb{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedOptimisticUpdateDeneb(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*optimisticUpdateDeneb)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *optimisticUpdateDeneb) Proto() proto.Message {
	return u.p
}

func (u *optimisticUpdateDeneb) Version() int {
	return slots.ToForkVersion(u.attestedHeader.Beacon().Slot)
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
