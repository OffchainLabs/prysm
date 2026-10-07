package light_client

import (
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	consensustypes "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"google.golang.org/protobuf/proto"
)

type bootstrapGloas struct {
	p                          *pb.LightClientBootstrapGloas
	header                     interfaces.LightClientHeader
	currentSyncCommitteeBranch interfaces.LightClientSyncCommitteeBranchGloas
}

var _ interfaces.LightClientBootstrap = &bootstrapGloas{}

func NewWrappedBootstrapGloas(p *pb.LightClientBootstrapGloas) (interfaces.LightClientBootstrap, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}

	var header interfaces.LightClientHeader
	var err error
	if p.Header != nil {
		header, err = NewWrappedHeaderGloas(p.Header)
		if err != nil {
			return nil, err
		}
	}

	branch, err := createBranch[interfaces.LightClientSyncCommitteeBranchGloas](
		"sync committee",
		p.CurrentSyncCommitteeBranch,
		fieldparams.SyncCommitteeBranchDepthGloas,
	)
	if err != nil {
		return nil, err
	}

	return &bootstrapGloas{
		p:                          p,
		header:                     header,
		currentSyncCommitteeBranch: branch,
	}, nil
}

func (h *bootstrapGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	return h.p.MarshalSSZTo(dst)
}

func (h *bootstrapGloas) MarshalSSZ() ([]byte, error) {
	return h.p.MarshalSSZ()
}

func (h *bootstrapGloas) SizeSSZ() int {
	return h.p.SizeSSZ()
}

func (h *bootstrapGloas) Version() int {
	return version.Gloas
}

func (h *bootstrapGloas) Proto() proto.Message {
	return h.p
}

func (h *bootstrapGloas) Header() interfaces.LightClientHeader {
	return h.header
}

func (h *bootstrapGloas) SetHeader(header interfaces.LightClientHeader) error {
	if header == nil {
		return consensustypes.ErrNilObjectWrapped
	}
	p, ok := header.Proto().(*pb.LightClientHeaderGloas)
	if !ok {
		return fmt.Errorf("header type %T is not %T", header.Proto(), &pb.LightClientHeaderGloas{})
	}
	h.p.Header = p
	h.header = header
	return nil
}

func (h *bootstrapGloas) CurrentSyncCommittee() *pb.SyncCommittee {
	return h.p.CurrentSyncCommittee
}

func (h *bootstrapGloas) SetCurrentSyncCommittee(sc *pb.SyncCommittee) error {
	h.p.CurrentSyncCommittee = sc
	return nil
}

func (h *bootstrapGloas) CurrentSyncCommitteeBranch() (interfaces.LightClientSyncCommitteeBranch, error) {
	return [5][32]byte{}, consensustypes.ErrNotSupported("CurrentSyncCommitteeBranch", version.Gloas)
}

func (h *bootstrapGloas) SetCurrentSyncCommitteeBranch(branch [][]byte) error {
	newBranch, err := createBranch[interfaces.LightClientSyncCommitteeBranchGloas]("sync committee", branch, fieldparams.SyncCommitteeBranchDepthGloas)
	if err != nil {
		return err
	}
	h.currentSyncCommitteeBranch = newBranch
	h.p.CurrentSyncCommitteeBranch = branch
	return nil
}

func (h *bootstrapGloas) CurrentSyncCommitteeBranchGloas() (interfaces.LightClientSyncCommitteeBranchGloas, error) {
	return h.currentSyncCommitteeBranch, nil
}

func (h *bootstrapGloas) CurrentSyncCommitteeBranchElectra() (interfaces.LightClientSyncCommitteeBranchElectra, error) {
	return interfaces.LightClientSyncCommitteeBranchElectra{}, consensustypes.ErrNotSupported("CurrentSyncCommitteeBranchElectra", version.Gloas)
}
