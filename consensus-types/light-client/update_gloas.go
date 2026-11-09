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

type updateGloas struct {
	p                       *pb.LightClientUpdateGloas
	attestedHeader          interfaces.LightClientHeader
	nextSyncCommitteeBranch interfaces.LightClientSyncCommitteeBranchGloas
	finalizedHeader         interfaces.LightClientHeader
	finalityBranch          interfaces.LightClientFinalityBranchGloas
}

var _ interfaces.LightClientUpdate = &updateGloas{}

func NewWrappedUpdateGloas(p *pb.LightClientUpdateGloas) (interfaces.LightClientUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}

	attestedHeader, err := NewWrappedHeaderGloas(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	var finalizedHeader interfaces.LightClientHeader
	if p.FinalizedHeader != nil {
		finalizedHeader, err = NewWrappedHeaderGloas(p.FinalizedHeader)
		if err != nil {
			return nil, err
		}
	}

	scBranch, err := createBranch[interfaces.LightClientSyncCommitteeBranchGloas](
		"sync committee",
		p.NextSyncCommitteeBranch,
		fieldparams.SyncCommitteeBranchDepthGloas,
	)
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

	return &updateGloas{
		p:                       p,
		attestedHeader:          attestedHeader,
		nextSyncCommitteeBranch: scBranch,
		finalizedHeader:         finalizedHeader,
		finalityBranch:          finalityBranch,
	}, nil
}

func (u *updateGloas) IsNil() bool {
	return u == nil || u.p == nil
}

func (u *updateGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *updateGloas) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *updateGloas) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *updateGloas) Proto() proto.Message {
	return u.p
}

func (u *updateGloas) Version() int {
	return version.Gloas
}

func (u *updateGloas) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *updateGloas) SetAttestedHeader(header interfaces.LightClientHeader) error {
	if header == nil {
		return consensustypes.ErrNilObjectWrapped
	}
	p, ok := header.Proto().(*pb.LightClientHeaderGloas)
	if !ok {
		return fmt.Errorf("header type %T is not %T", header.Proto(), &pb.LightClientHeaderGloas{})
	}
	u.p.AttestedHeader = p
	u.attestedHeader = header
	return nil
}

func (u *updateGloas) NextSyncCommittee() *pb.SyncCommittee {
	return u.p.NextSyncCommittee
}

func (u *updateGloas) SetNextSyncCommittee(sc *pb.SyncCommittee) {
	u.p.NextSyncCommittee = sc
}

func (u *updateGloas) NextSyncCommitteeBranch() (interfaces.LightClientSyncCommitteeBranch, error) {
	return [5][32]byte{}, consensustypes.ErrNotSupported("NextSyncCommitteeBranch", version.Gloas)
}

func (u *updateGloas) SetNextSyncCommitteeBranch(branch [][]byte) error {
	b, err := createBranch[interfaces.LightClientSyncCommitteeBranchGloas]("sync committee", branch, fieldparams.SyncCommitteeBranchDepthGloas)
	if err != nil {
		return err
	}
	u.nextSyncCommitteeBranch = b

	u.p.NextSyncCommitteeBranch = branch

	return nil
}

func (u *updateGloas) NextSyncCommitteeBranchGloas() (interfaces.LightClientSyncCommitteeBranchGloas, error) {
	return u.nextSyncCommitteeBranch, nil
}

func (u *updateGloas) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *updateGloas) SetFinalizedHeader(header interfaces.LightClientHeader) error {
	if header == nil {
		return consensustypes.ErrNilObjectWrapped
	}
	p, ok := header.Proto().(*pb.LightClientHeaderGloas)
	if !ok {
		return fmt.Errorf("header type %T is not %T", header.Proto(), &pb.LightClientHeaderGloas{})
	}
	u.p.FinalizedHeader = p
	u.finalizedHeader = header
	return nil
}

func (u *updateGloas) FinalityBranch() (interfaces.LightClientFinalityBranch, error) {
	return interfaces.LightClientFinalityBranch{}, consensustypes.ErrNotSupported("FinalityBranch", u.Version())
}

func (u *updateGloas) FinalityBranchGloas() (interfaces.LightClientFinalityBranchGloas, error) {
	return u.finalityBranch, nil
}

func (u *updateGloas) SetFinalityBranch(branch [][]byte) error {
	b, err := createBranch[interfaces.LightClientFinalityBranchGloas]("finality", branch, fieldparams.FinalityBranchDepthGloas)
	if err != nil {
		return err
	}
	u.finalityBranch = b
	u.p.FinalityBranch = branch
	return nil
}

func (u *updateGloas) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *updateGloas) SetSyncAggregate(sa *pb.SyncAggregate) {
	u.p.SyncAggregate = sa
}

func (u *updateGloas) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

func (u *updateGloas) SetSignatureSlot(slot primitives.Slot) {
	u.p.SignatureSlot = slot
}

func (u *updateGloas) NextSyncCommitteeBranchElectra() (interfaces.LightClientSyncCommitteeBranchElectra, error) {
	return interfaces.LightClientSyncCommitteeBranchElectra{}, consensustypes.ErrNotSupported("NextSyncCommitteeBranchElectra", version.Gloas)
}

func (u *updateGloas) FinalityBranchElectra() (interfaces.LightClientFinalityBranchElectra, error) {
	return interfaces.LightClientFinalityBranchElectra{}, consensustypes.ErrNotSupported("FinalityBranchElectra", version.Gloas)
}
