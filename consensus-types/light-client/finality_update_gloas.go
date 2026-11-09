package light_client

import (
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	consensustypes "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"google.golang.org/protobuf/proto"
)

type finalityUpdateGloas struct {
	p               *pb.LightClientFinalityUpdateGloas
	attestedHeader  interfaces.LightClientHeader
	finalizedHeader interfaces.LightClientHeader
	finalityBranch  interfaces.LightClientFinalityBranchGloas
}

// NewEmptyFinalityUpdateGloas creates an empty wrapper for SSZ unmarshalling.
func NewEmptyFinalityUpdateGloas() interfaces.LightClientFinalityUpdate {
	return &finalityUpdateGloas{}
}

func (u *finalityUpdateGloas) IsNil() bool {
	return u == nil || u.p == nil
}

var _ interfaces.LightClientFinalityUpdate = &finalityUpdateGloas{}

func NewWrappedFinalityUpdateGloas(p *pb.LightClientFinalityUpdateGloas) (interfaces.LightClientFinalityUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderGloas(p.AttestedHeader)
	if err != nil {
		return nil, err
	}
	finalizedHeader, err := NewWrappedHeaderGloas(p.FinalizedHeader)
	if err != nil {
		return nil, err
	}

	finalityBranch, err := createBranch[interfaces.LightClientFinalityBranchGloas](
		"finality",
		p.FinalityBranch,
		fieldparams.FinalityBranchDepthGloas,
	)
	if err != nil {
		return nil, err
	}

	return &finalityUpdateGloas{
		p:               p,
		attestedHeader:  attestedHeader,
		finalizedHeader: finalizedHeader,
		finalityBranch:  finalityBranch,
	}, nil
}

func (u *finalityUpdateGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *finalityUpdateGloas) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *finalityUpdateGloas) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *finalityUpdateGloas) UnmarshalSSZ(buf []byte) error {
	p := &pb.LightClientFinalityUpdateGloas{}
	if err := p.UnmarshalSSZ(buf); err != nil {
		return err
	}
	updateInterface, err := NewWrappedFinalityUpdateGloas(p)
	if err != nil {
		return err
	}
	update, ok := updateInterface.(*finalityUpdateGloas)
	if !ok {
		return fmt.Errorf("unexpected update type %T", updateInterface)
	}
	*u = *update
	return nil
}

func (u *finalityUpdateGloas) Proto() proto.Message {
	return u.p
}

func (u *finalityUpdateGloas) Version() int {
	return version.Gloas
}

func (u *finalityUpdateGloas) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *finalityUpdateGloas) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *finalityUpdateGloas) FinalityBranch() (interfaces.LightClientFinalityBranch, error) {
	return interfaces.LightClientFinalityBranch{}, consensustypes.ErrNotSupported("FinalityBranch", u.Version())
}

func (u *finalityUpdateGloas) FinalityBranchGloas() (interfaces.LightClientFinalityBranchGloas, error) {
	return u.finalityBranch, nil
}

func (u *finalityUpdateGloas) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *finalityUpdateGloas) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

func (u *finalityUpdateGloas) FinalityBranchElectra() (interfaces.LightClientFinalityBranchElectra, error) {
	return interfaces.LightClientFinalityBranchElectra{}, consensustypes.ErrNotSupported("FinalityBranchElectra", version.Gloas)
}
