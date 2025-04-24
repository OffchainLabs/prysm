package light_client

import (
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	consensustypes "github.com/OffchainLabs/prysm/v6/consensus-types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
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
		return &FinalityUpdateAltair{
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
		return &FinalityUpdateCapella{
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
		return &FinalityUpdateDeneb{
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
		return &FinalityUpdateElectra{
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
type FinalityUpdateAltair struct {
	p               *pb.LightClientFinalityUpdateAltair
	attestedHeader  interfaces.LightClientHeader
	finalizedHeader interfaces.LightClientHeader
	finalityBranch  interfaces.LightClientFinalityBranch
}

func (u *FinalityUpdateAltair) IsNil() bool {
	return u == nil || u.p == nil
}

var _ interfaces.LightClientFinalityUpdate = &FinalityUpdateAltair{}

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

	return &FinalityUpdateAltair{
		p:               p,
		attestedHeader:  attestedHeader,
		finalizedHeader: finalizedHeader,
		finalityBranch:  branch,
	}, nil
}

func (u *FinalityUpdateAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *FinalityUpdateAltair) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *FinalityUpdateAltair) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *FinalityUpdateAltair) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientFinalityUpdateAltair{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedFinalityUpdateAltair(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*FinalityUpdateAltair)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *FinalityUpdateAltair) Proto() proto.Message {
	return u.p
}

func (u *FinalityUpdateAltair) Version() int {
	return version.Altair
}

func (u *FinalityUpdateAltair) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *FinalityUpdateAltair) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *FinalityUpdateAltair) FinalityBranch() (interfaces.LightClientFinalityBranch, error) {
	return u.finalityBranch, nil
}

func (u *FinalityUpdateAltair) FinalityBranchElectra() (interfaces.LightClientFinalityBranchElectra, error) {
	return interfaces.LightClientFinalityBranchElectra{}, consensustypes.ErrNotSupported("FinalityBranchElectra", u.Version())
}

func (u *FinalityUpdateAltair) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *FinalityUpdateAltair) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
type FinalityUpdateCapella struct {
	p               *pb.LightClientFinalityUpdateCapella
	attestedHeader  interfaces.LightClientHeader
	finalizedHeader interfaces.LightClientHeader
	finalityBranch  interfaces.LightClientFinalityBranch
}

func (u *FinalityUpdateCapella) IsNil() bool {
	return u == nil || u.p == nil
}

var _ interfaces.LightClientFinalityUpdate = &FinalityUpdateCapella{}

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

	return &FinalityUpdateCapella{
		p:               p,
		attestedHeader:  attestedHeader,
		finalizedHeader: finalizedHeader,
		finalityBranch:  branch,
	}, nil
}

func (u *FinalityUpdateCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *FinalityUpdateCapella) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *FinalityUpdateCapella) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *FinalityUpdateCapella) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientFinalityUpdateCapella{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedFinalityUpdateCapella(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*FinalityUpdateCapella)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *FinalityUpdateCapella) Proto() proto.Message {
	return u.p
}

func (u *FinalityUpdateCapella) Version() int {
	return version.Capella
}

func (u *FinalityUpdateCapella) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *FinalityUpdateCapella) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *FinalityUpdateCapella) FinalityBranch() (interfaces.LightClientFinalityBranch, error) {
	return u.finalityBranch, nil
}

func (u *FinalityUpdateCapella) FinalityBranchElectra() (interfaces.LightClientFinalityBranchElectra, error) {
	return interfaces.LightClientFinalityBranchElectra{}, consensustypes.ErrNotSupported("FinalityBranchElectra", u.Version())
}

func (u *FinalityUpdateCapella) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *FinalityUpdateCapella) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
type FinalityUpdateDeneb struct {
	p               *pb.LightClientFinalityUpdateDeneb
	attestedHeader  interfaces.LightClientHeader
	finalizedHeader interfaces.LightClientHeader
	finalityBranch  interfaces.LightClientFinalityBranch
}

func (u *FinalityUpdateDeneb) IsNil() bool {
	return u == nil || u.p == nil
}

var _ interfaces.LightClientFinalityUpdate = &FinalityUpdateDeneb{}

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

	return &FinalityUpdateDeneb{
		p:               p,
		attestedHeader:  attestedHeader,
		finalizedHeader: finalizedHeader,
		finalityBranch:  branch,
	}, nil
}

func (u *FinalityUpdateDeneb) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *FinalityUpdateDeneb) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *FinalityUpdateDeneb) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *FinalityUpdateDeneb) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientFinalityUpdateDeneb{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedFinalityUpdateDeneb(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*FinalityUpdateDeneb)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *FinalityUpdateDeneb) Proto() proto.Message {
	return u.p
}

func (u *FinalityUpdateDeneb) Version() int {
	return version.Deneb
}

func (u *FinalityUpdateDeneb) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *FinalityUpdateDeneb) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *FinalityUpdateDeneb) FinalityBranch() (interfaces.LightClientFinalityBranch, error) {
	return u.finalityBranch, nil
}

func (u *FinalityUpdateDeneb) FinalityBranchElectra() (interfaces.LightClientFinalityBranchElectra, error) {
	return interfaces.LightClientFinalityBranchElectra{}, consensustypes.ErrNotSupported("FinalityBranchElectra", u.Version())
}

func (u *FinalityUpdateDeneb) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *FinalityUpdateDeneb) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

// In addition to the proto object being wrapped, we store some fields that have to be
// constructed from the proto, so that we don't have to reconstruct them every time
// in getters.
type FinalityUpdateElectra struct {
	p               *pb.LightClientFinalityUpdateElectra
	attestedHeader  interfaces.LightClientHeader
	finalizedHeader interfaces.LightClientHeader
	finalityBranch  interfaces.LightClientFinalityBranchElectra
}

func (u *FinalityUpdateElectra) IsNil() bool {
	return u == nil || u.p == nil
}

var _ interfaces.LightClientFinalityUpdate = &FinalityUpdateElectra{}

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

	return &FinalityUpdateElectra{
		p:               p,
		attestedHeader:  attestedHeader,
		finalizedHeader: finalizedHeader,
		finalityBranch:  finalityBranch,
	}, nil
}

func (u *FinalityUpdateElectra) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *FinalityUpdateElectra) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *FinalityUpdateElectra) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *FinalityUpdateElectra) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientFinalityUpdateElectra{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedFinalityUpdateElectra(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*FinalityUpdateElectra)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *FinalityUpdateElectra) Proto() proto.Message {
	return u.p
}

func (u *FinalityUpdateElectra) Version() int {
	return version.Electra
}

func (u *FinalityUpdateElectra) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *FinalityUpdateElectra) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *FinalityUpdateElectra) FinalityBranch() (interfaces.LightClientFinalityBranch, error) {
	return interfaces.LightClientFinalityBranch{}, consensustypes.ErrNotSupported("FinalityBranch", u.Version())
}

func (u *FinalityUpdateElectra) FinalityBranchElectra() (interfaces.LightClientFinalityBranchElectra, error) {
	return u.finalityBranch, nil
}

func (u *FinalityUpdateElectra) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *FinalityUpdateElectra) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}
