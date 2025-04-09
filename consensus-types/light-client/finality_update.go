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
	case *pb.LightClientFinalityUpdateElectra:
		return NewWrappedFinalityUpdateElectra(t)
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
		return &finalityUpdateElectra{
			p: &pb.LightClientFinalityUpdateElectra{
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

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
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
	attestedHeader, err := NewWrappedHeader(p.AttestedHeader)
	if err != nil {
		return nil, err
	}
	finalizedHeader, err := NewWrappedHeader(p.FinalizedHeader)
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

func (u *finalityUpdateAltair) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientFinalityUpdateAltair{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedFinalityUpdateAltair(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*finalityUpdateAltair)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
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

func (u *finalityUpdateAltair) FinalityBranch() (interfaces.LightClientFinalityBranch, error) {
	return u.finalityBranch, nil
}

func (u *finalityUpdateAltair) FinalityBranchElectra() (interfaces.LightClientFinalityBranchElectra, error) {
	return interfaces.LightClientFinalityBranchElectra{}, consensustypes.ErrNotSupported("FinalityBranchElectra", u.Version())
}

func (u *finalityUpdateAltair) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *finalityUpdateAltair) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
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
	attestedHeader, err := NewWrappedHeader(p.AttestedHeader)
	if err != nil {
		return nil, err
	}
	finalizedHeader, err := NewWrappedHeader(p.FinalizedHeader)
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

func (u *finalityUpdateCapella) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientFinalityUpdateCapella{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedFinalityUpdateCapella(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*finalityUpdateCapella)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
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

func (u *finalityUpdateCapella) FinalityBranch() (interfaces.LightClientFinalityBranch, error) {
	return u.finalityBranch, nil
}

func (u *finalityUpdateCapella) FinalityBranchElectra() (interfaces.LightClientFinalityBranchElectra, error) {
	return interfaces.LightClientFinalityBranchElectra{}, consensustypes.ErrNotSupported("FinalityBranchElectra", u.Version())
}

func (u *finalityUpdateCapella) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *finalityUpdateCapella) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
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
	attestedHeader, err := NewWrappedHeader(p.AttestedHeader)
	if err != nil {
		return nil, err
	}
	finalizedHeader, err := NewWrappedHeader(p.FinalizedHeader)
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

func (u *finalityUpdateDeneb) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientFinalityUpdateDeneb{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedFinalityUpdateDeneb(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*finalityUpdateDeneb)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
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

func (u *finalityUpdateDeneb) FinalityBranch() (interfaces.LightClientFinalityBranch, error) {
	return u.finalityBranch, nil
}

func (u *finalityUpdateDeneb) FinalityBranchElectra() (interfaces.LightClientFinalityBranchElectra, error) {
	return interfaces.LightClientFinalityBranchElectra{}, consensustypes.ErrNotSupported("FinalityBranchElectra", u.Version())
}

func (u *finalityUpdateDeneb) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *finalityUpdateDeneb) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
type finalityUpdateElectra struct {
	p               *pb.LightClientFinalityUpdateElectra
	attestedHeader  interfaces.LightClientHeader
	finalizedHeader interfaces.LightClientHeader
	finalityBranch  interfaces.LightClientFinalityBranchElectra
}

var _ interfaces.LightClientFinalityUpdate = &finalityUpdateElectra{}

func NewWrappedFinalityUpdateElectra(p *pb.LightClientFinalityUpdateElectra) (interfaces.LightClientFinalityUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeader(p.AttestedHeader)
	if err != nil {
		return nil, err
	}
	finalizedHeader, err := NewWrappedHeader(p.FinalizedHeader)
	if err != nil {
		return nil, err
	}

	finalityBranch, err := createBranch[interfaces.LightClientFinalityBranchElectra](
		"finality",
		p.FinalityBranch,
		fieldparams.FinalityBranchDepthElectra,
	)
	if err != nil {
		return nil, err
	}

	return &finalityUpdateElectra{
		p:               p,
		attestedHeader:  attestedHeader,
		finalizedHeader: finalizedHeader,
		finalityBranch:  finalityBranch,
	}, nil
}

func (u *finalityUpdateElectra) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *finalityUpdateElectra) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *finalityUpdateElectra) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *finalityUpdateElectra) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientFinalityUpdateElectra{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedFinalityUpdateElectra(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*finalityUpdateElectra)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *finalityUpdateElectra) Proto() proto.Message {
	return u.p
}

func (u *finalityUpdateElectra) Version() int {
	return version.Electra
}

func (u *finalityUpdateElectra) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *finalityUpdateElectra) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *finalityUpdateElectra) FinalityBranch() (interfaces.LightClientFinalityBranch, error) {
	return interfaces.LightClientFinalityBranch{}, consensustypes.ErrNotSupported("FinalityBranch", u.Version())
}

func (u *finalityUpdateElectra) FinalityBranchElectra() (interfaces.LightClientFinalityBranchElectra, error) {
	return u.finalityBranch, nil
}

func (u *finalityUpdateElectra) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *finalityUpdateElectra) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}
