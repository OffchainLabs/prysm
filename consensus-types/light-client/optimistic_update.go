package light_client

import (
	"fmt"

	consensustypes "github.com/OffchainLabs/prysm/v6/consensus-types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
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
		return &OptimisticUpdateAltair{
			p: &pb.LightClientOptimisticUpdateAltair{
				AttestedHeader: t.p.AttestedHeader,
				SyncAggregate:  t.p.SyncAggregate,
				SignatureSlot:  t.p.SignatureSlot,
			},
			attestedHeader: t.attestedHeader,
		}, nil
	case *updateCapella:
		return &OptimisticUpdateCapella{
			p: &pb.LightClientOptimisticUpdateCapella{
				AttestedHeader: t.p.AttestedHeader,
				SyncAggregate:  t.p.SyncAggregate,
				SignatureSlot:  t.p.SignatureSlot,
			},
			attestedHeader: t.attestedHeader,
		}, nil
	case *updateDeneb:
		return &OptimisticUpdateDeneb{
			p: &pb.LightClientOptimisticUpdateDeneb{
				AttestedHeader: t.p.AttestedHeader,
				SyncAggregate:  t.p.SyncAggregate,
				SignatureSlot:  t.p.SignatureSlot,
			},
			attestedHeader: t.attestedHeader,
		}, nil
	case *updateElectra:
		return &OptimisticUpdateDeneb{
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
type OptimisticUpdateAltair struct {
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

var _ interfaces.LightClientOptimisticUpdate = &OptimisticUpdateAltair{}

func NewWrappedOptimisticUpdateAltair(p *pb.LightClientOptimisticUpdateAltair) (interfaces.LightClientOptimisticUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderAltair(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	return &OptimisticUpdateAltair{
		p:              p,
		attestedHeader: attestedHeader,
	}, nil
}

func (u *OptimisticUpdateAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *OptimisticUpdateAltair) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *OptimisticUpdateAltair) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *OptimisticUpdateAltair) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientOptimisticUpdateAltair{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedOptimisticUpdateAltair(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*OptimisticUpdateAltair)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *OptimisticUpdateAltair) Proto() proto.Message {
	return u.p
}

func (u *OptimisticUpdateAltair) Version() int {
	return version.Altair
}

func (u *OptimisticUpdateAltair) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *OptimisticUpdateAltair) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *OptimisticUpdateAltair) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
type OptimisticUpdateCapella struct {
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

var _ interfaces.LightClientOptimisticUpdate = &OptimisticUpdateCapella{}

func NewWrappedOptimisticUpdateCapella(p *pb.LightClientOptimisticUpdateCapella) (interfaces.LightClientOptimisticUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderCapella(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	return &OptimisticUpdateCapella{
		p:              p,
		attestedHeader: attestedHeader,
	}, nil
}

func (u *OptimisticUpdateCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *OptimisticUpdateCapella) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *OptimisticUpdateCapella) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *OptimisticUpdateCapella) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientOptimisticUpdateCapella{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedOptimisticUpdateCapella(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*OptimisticUpdateCapella)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *OptimisticUpdateCapella) Proto() proto.Message {
	return u.p
}

func (u *OptimisticUpdateCapella) Version() int {
	return version.Capella
}

func (u *OptimisticUpdateCapella) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *OptimisticUpdateCapella) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *OptimisticUpdateCapella) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
type OptimisticUpdateDeneb struct {
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

var _ interfaces.LightClientOptimisticUpdate = &OptimisticUpdateDeneb{}

func NewWrappedOptimisticUpdateDeneb(p *pb.LightClientOptimisticUpdateDeneb) (interfaces.LightClientOptimisticUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderDeneb(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	return &OptimisticUpdateDeneb{
		p:              p,
		attestedHeader: attestedHeader,
	}, nil
}

func (u *OptimisticUpdateDeneb) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *OptimisticUpdateDeneb) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *OptimisticUpdateDeneb) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *OptimisticUpdateDeneb) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientOptimisticUpdateDeneb{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedOptimisticUpdateDeneb(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*OptimisticUpdateDeneb)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *OptimisticUpdateDeneb) Proto() proto.Message {
	return u.p
}

func (u *OptimisticUpdateDeneb) Version() int {
	return version.Deneb
}

func (u *OptimisticUpdateDeneb) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *OptimisticUpdateDeneb) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *OptimisticUpdateDeneb) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}
