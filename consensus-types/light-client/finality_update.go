package light_client

import (
	"fmt"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	consensustypes "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"google.golang.org/protobuf/proto"
)

func NewWrappedFinalityUpdate(m proto.Message) (interfaces.LightClientFinalityUpdate, error) {
	if m == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	switch t := m.(type) {
	case *pb.LightClientFinalityUpdateAltair:
		return NewWrappedFinalityUpdateAltair(t)
	case *pb.LightClientFinalityUpdateCapella:
		return NewWrappedFinalityUpdateCapella(t)
	case *pb.LightClientFinalityUpdateDeneb:
		return NewWrappedFinalityUpdateDeneb(t)
	default:
		return nil, fmt.Errorf("cannot construct light client finality update from type %T", t)
	}
}

func NewFinalityUpdateFromUpdate(update interfaces.LightClientUpdate) (interfaces.LightClientFinalityUpdate, error) {
	switch t := update.(type) {
	case *updateAltair:
		return &finalityUpdateAltair{
			p: &pb.LightClientFinalityUpdateAltair{
				AttestedHeader:  t.p.AttestedHeader,
				FinalizedHeader: t.p.FinalizedHeader,
				FinalityBranch:  t.p.FinalityBranch,
				SyncAggregate:   t.p.SyncAggregate,
				SignatureSlot:   t.p.SignatureSlot,
			},
			attestedHeader:  t.attestedHeader,
			finalizedHeader: t.finalizedHeader,
			finalityBranch:  t.finalityBranch,
		}, nil
	case *updateCapella:
		return &finalityUpdateCapella{
			p: &pb.LightClientFinalityUpdateCapella{
				AttestedHeader:  t.p.AttestedHeader,
				FinalizedHeader: t.p.FinalizedHeader,
				FinalityBranch:  t.p.FinalityBranch,
				SyncAggregate:   t.p.SyncAggregate,
				SignatureSlot:   t.p.SignatureSlot,
			},
			attestedHeader:  t.attestedHeader,
			finalizedHeader: t.finalizedHeader,
			finalityBranch:  t.finalityBranch,
		}, nil
	case *updateDeneb:
		return &finalityUpdateDeneb{
			p: &pb.LightClientFinalityUpdateDeneb{
				AttestedHeader:  t.p.AttestedHeader,
				FinalizedHeader: t.p.FinalizedHeader,
				FinalityBranch:  t.p.FinalityBranch,
				SyncAggregate:   t.p.SyncAggregate,
				SignatureSlot:   t.p.SignatureSlot,
			},
			attestedHeader:  t.attestedHeader,
			finalizedHeader: t.finalizedHeader,
			finalityBranch:  t.finalityBranch,
		}, nil
	case *updateElectra:
		return &finalityUpdateDeneb{
			p: &pb.LightClientFinalityUpdateDeneb{
				AttestedHeader:  t.p.AttestedHeader,
				FinalizedHeader: t.p.FinalizedHeader,
				FinalityBranch:  t.p.FinalityBranch,
				SyncAggregate:   t.p.SyncAggregate,
				SignatureSlot:   t.p.SignatureSlot,
			},
			attestedHeader:  t.attestedHeader,
			finalizedHeader: t.finalizedHeader,
			finalityBranch:  t.finalityBranch,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported type %T", t)
	}
}

type finalityUpdateAltair struct {
	p               *pb.LightClientFinalityUpdateAltair
	attestedHeader  interfaces.LightClientHeader
	finalizedHeader interfaces.LightClientHeader
	finalityBranch  interfaces.LightClientFinalityBranch
}

var _ interfaces.LightClientFinalityUpdate = &finalityUpdateAltair{}

func NewWrappedFinalityUpdateAltair(p *pb.LightClientFinalityUpdateAltair) (interfaces.LightClientFinalityUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderAltair(p.AttestedHeader)
	if err != nil {
		return nil, err
	}
	finalizedHeader, err := NewWrappedHeaderAltair(p.FinalizedHeader)
	if err != nil {
		return nil, err
	}
	branch, err := createBranch[interfaces.LightClientFinalityBranch](
		"finality",
		p.FinalityBranch,
		fieldparams.FinalityBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &finalityUpdateAltair{
		p:               p,
		attestedHeader:  attestedHeader,
		finalizedHeader: finalizedHeader,
		finalityBranch:  branch,
	}, nil
}

func (u *finalityUpdateAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *finalityUpdateAltair) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *finalityUpdateAltair) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *finalityUpdateAltair) Proto() proto.Message {
	return u.p
}

func (u *finalityUpdateAltair) Version() int {
	return version.Altair
}

func (u *finalityUpdateAltair) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *finalityUpdateAltair) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *finalityUpdateAltair) FinalityBranch() interfaces.LightClientFinalityBranch {
	return u.finalityBranch
}

func (u *finalityUpdateAltair) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *finalityUpdateAltair) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

type finalityUpdateCapella struct {
	p               *pb.LightClientFinalityUpdateCapella
	attestedHeader  interfaces.LightClientHeader
	finalizedHeader interfaces.LightClientHeader
	finalityBranch  interfaces.LightClientFinalityBranch
}

var _ interfaces.LightClientFinalityUpdate = &finalityUpdateCapella{}

func NewWrappedFinalityUpdateCapella(p *pb.LightClientFinalityUpdateCapella) (interfaces.LightClientFinalityUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderCapella(p.AttestedHeader)
	if err != nil {
		return nil, err
	}
	finalizedHeader, err := NewWrappedHeaderCapella(p.FinalizedHeader)
	if err != nil {
		return nil, err
	}
	branch, err := createBranch[interfaces.LightClientFinalityBranch](
		"finality",
		p.FinalityBranch,
		fieldparams.FinalityBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &finalityUpdateCapella{
		p:               p,
		attestedHeader:  attestedHeader,
		finalizedHeader: finalizedHeader,
		finalityBranch:  branch,
	}, nil
}

func (u *finalityUpdateCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *finalityUpdateCapella) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *finalityUpdateCapella) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *finalityUpdateCapella) Proto() proto.Message {
	return u.p
}

func (u *finalityUpdateCapella) Version() int {
	return version.Capella
}

func (u *finalityUpdateCapella) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *finalityUpdateCapella) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *finalityUpdateCapella) FinalityBranch() interfaces.LightClientFinalityBranch {
	return u.finalityBranch
}

func (u *finalityUpdateCapella) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *finalityUpdateCapella) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

type finalityUpdateDeneb struct {
	p               *pb.LightClientFinalityUpdateDeneb
	attestedHeader  interfaces.LightClientHeader
	finalizedHeader interfaces.LightClientHeader
	finalityBranch  interfaces.LightClientFinalityBranch
}

var _ interfaces.LightClientFinalityUpdate = &finalityUpdateDeneb{}

func NewWrappedFinalityUpdateDeneb(p *pb.LightClientFinalityUpdateDeneb) (interfaces.LightClientFinalityUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderDeneb(p.AttestedHeader)
	if err != nil {
		return nil, err
	}
	finalizedHeader, err := NewWrappedHeaderDeneb(p.FinalizedHeader)
	if err != nil {
		return nil, err
	}
	branch, err := createBranch[interfaces.LightClientFinalityBranch](
		"finality",
		p.FinalityBranch,
		fieldparams.FinalityBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &finalityUpdateDeneb{
		p:               p,
		attestedHeader:  attestedHeader,
		finalizedHeader: finalizedHeader,
		finalityBranch:  branch,
	}, nil
}

func (u *finalityUpdateDeneb) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *finalityUpdateDeneb) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *finalityUpdateDeneb) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *finalityUpdateDeneb) Proto() proto.Message {
	return u.p
}

func (u *finalityUpdateDeneb) Version() int {
	return version.Deneb
}

func (u *finalityUpdateDeneb) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *finalityUpdateDeneb) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *finalityUpdateDeneb) FinalityBranch() interfaces.LightClientFinalityBranch {
	return u.finalityBranch
}

func (u *finalityUpdateDeneb) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *finalityUpdateDeneb) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}
