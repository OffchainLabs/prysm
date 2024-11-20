package light_client

import (
	"fmt"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	consensustypes "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"google.golang.org/protobuf/proto"
)

func NewWrappedBootstrap(m proto.Message) (interfaces.LightClientBootstrap, error) {
	if m == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	switch t := m.(type) {
	case *pb.LightClientBootstrapAltair:
		return NewWrappedBootstrapAltair(t)
	case *pb.LightClientBootstrapCapella:
		return NewWrappedBootstrapCapella(t)
	case *pb.LightClientBootstrapDeneb:
		return NewWrappedBootstrapDeneb(t)
	case *pb.LightClientBootstrapElectra:
		return NewWrappedBootstrapElectra(t)
	default:
		return nil, fmt.Errorf("cannot construct light client bootstrap from type %T", t)
	}
}

type bootstrapAltair struct {
	p                          *pb.LightClientBootstrapAltair
	header                     interfaces.LightClientHeader
	currentSyncCommitteeBranch interfaces.LightClientSyncCommitteeBranch
}

var _ interfaces.LightClientBootstrap = &bootstrapAltair{}

func NewWrappedBootstrapAltair(p *pb.LightClientBootstrapAltair) (interfaces.LightClientBootstrap, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	header, err := NewWrappedHeader(p.Header)
	if err != nil {
		return nil, err
	}
	branch, err := createBranch[interfaces.LightClientSyncCommitteeBranch](
		"sync committee",
		p.CurrentSyncCommitteeBranch,
		fieldparams.SyncCommitteeBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &bootstrapAltair{
		p:                          p,
		header:                     header,
		currentSyncCommitteeBranch: branch,
	}, nil
}

func (h *bootstrapAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	return h.p.MarshalSSZTo(dst)
}

func (h *bootstrapAltair) MarshalSSZ() ([]byte, error) {
	return h.p.MarshalSSZ()
}

func (h *bootstrapAltair) SizeSSZ() int {
	return h.p.SizeSSZ()
}

func (h *bootstrapAltair) Version() int {
	return version.Altair
}

func (h *bootstrapAltair) Header() interfaces.LightClientHeader {
	return h.header
}

func (h *bootstrapAltair) CurrentSyncCommittee() *pb.SyncCommittee {
	return h.p.CurrentSyncCommittee
}

func (h *bootstrapAltair) CurrentSyncCommitteeBranch() (interfaces.LightClientSyncCommitteeBranch, error) {
	return h.currentSyncCommitteeBranch, nil
}

func (h *bootstrapAltair) CurrentSyncCommitteeBranchElectra() (interfaces.LightClientSyncCommitteeBranchElectra, error) {
	return [6][32]byte{}, consensustypes.ErrNotSupported("CurrentSyncCommitteeBranchElectra", version.Altair)
}

type bootstrapCapella struct {
	p                          *pb.LightClientBootstrapCapella
	header                     interfaces.LightClientHeader
	currentSyncCommitteeBranch interfaces.LightClientSyncCommitteeBranch
}

var _ interfaces.LightClientBootstrap = &bootstrapCapella{}

func NewWrappedBootstrapCapella(p *pb.LightClientBootstrapCapella) (interfaces.LightClientBootstrap, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	header, err := NewWrappedHeader(p.Header)
	if err != nil {
		return nil, err
	}
	branch, err := createBranch[interfaces.LightClientSyncCommitteeBranch](
		"sync committee",
		p.CurrentSyncCommitteeBranch,
		fieldparams.SyncCommitteeBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &bootstrapCapella{
		p:                          p,
		header:                     header,
		currentSyncCommitteeBranch: branch,
	}, nil
}

func (h *bootstrapCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	return h.p.MarshalSSZTo(dst)
}

func (h *bootstrapCapella) MarshalSSZ() ([]byte, error) {
	return h.p.MarshalSSZ()
}

func (h *bootstrapCapella) SizeSSZ() int {
	return h.p.SizeSSZ()
}

func (h *bootstrapCapella) Version() int {
	return version.Capella
}

func (h *bootstrapCapella) Header() interfaces.LightClientHeader {
	return h.header
}

func (h *bootstrapCapella) CurrentSyncCommittee() *pb.SyncCommittee {
	return h.p.CurrentSyncCommittee
}

func (h *bootstrapCapella) CurrentSyncCommitteeBranch() (interfaces.LightClientSyncCommitteeBranch, error) {
	return h.currentSyncCommitteeBranch, nil
}

func (h *bootstrapCapella) CurrentSyncCommitteeBranchElectra() (interfaces.LightClientSyncCommitteeBranchElectra, error) {
	return [6][32]byte{}, consensustypes.ErrNotSupported("CurrentSyncCommitteeBranchElectra", version.Capella)
}

type bootstrapDeneb struct {
	p                          *pb.LightClientBootstrapDeneb
	header                     interfaces.LightClientHeader
	currentSyncCommitteeBranch interfaces.LightClientSyncCommitteeBranch
}

var _ interfaces.LightClientBootstrap = &bootstrapDeneb{}

func NewWrappedBootstrapDeneb(p *pb.LightClientBootstrapDeneb) (interfaces.LightClientBootstrap, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	header, err := NewWrappedHeader(p.Header)
	if err != nil {
		return nil, err
	}
	branch, err := createBranch[interfaces.LightClientSyncCommitteeBranch](
		"sync committee",
		p.CurrentSyncCommitteeBranch,
		fieldparams.SyncCommitteeBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &bootstrapDeneb{
		p:                          p,
		header:                     header,
		currentSyncCommitteeBranch: branch,
	}, nil
}

func (h *bootstrapDeneb) MarshalSSZTo(dst []byte) ([]byte, error) {
	return h.p.MarshalSSZTo(dst)
}

func (h *bootstrapDeneb) MarshalSSZ() ([]byte, error) {
	return h.p.MarshalSSZ()
}

func (h *bootstrapDeneb) SizeSSZ() int {
	return h.p.SizeSSZ()
}

func (h *bootstrapDeneb) Version() int {
	return version.Deneb
}

func (h *bootstrapDeneb) Header() interfaces.LightClientHeader {
	return h.header
}

func (h *bootstrapDeneb) CurrentSyncCommittee() *pb.SyncCommittee {
	return h.p.CurrentSyncCommittee
}

func (h *bootstrapDeneb) CurrentSyncCommitteeBranch() (interfaces.LightClientSyncCommitteeBranch, error) {
	return h.currentSyncCommitteeBranch, nil
}

func (h *bootstrapDeneb) CurrentSyncCommitteeBranchElectra() (interfaces.LightClientSyncCommitteeBranchElectra, error) {
	return [6][32]byte{}, consensustypes.ErrNotSupported("CurrentSyncCommitteeBranchElectra", version.Deneb)
}

type bootstrapElectra struct {
	p                          *pb.LightClientBootstrapElectra
	header                     interfaces.LightClientHeader
	currentSyncCommitteeBranch interfaces.LightClientSyncCommitteeBranchElectra
}

var _ interfaces.LightClientBootstrap = &bootstrapElectra{}

func NewWrappedBootstrapElectra(p *pb.LightClientBootstrapElectra) (interfaces.LightClientBootstrap, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	header, err := NewWrappedHeader(p.Header)
	if err != nil {
		return nil, err
	}
	branch, err := createBranch[interfaces.LightClientSyncCommitteeBranchElectra](
		"sync committee",
		p.CurrentSyncCommitteeBranch,
		fieldparams.SyncCommitteeBranchDepthElectra,
	)
	if err != nil {
		return nil, err
	}

	return &bootstrapElectra{
		p:                          p,
		header:                     header,
		currentSyncCommitteeBranch: branch,
	}, nil
}

func (h *bootstrapElectra) MarshalSSZTo(dst []byte) ([]byte, error) {
	return h.p.MarshalSSZTo(dst)
}

func (h *bootstrapElectra) MarshalSSZ() ([]byte, error) {
	return h.p.MarshalSSZ()
}

func (h *bootstrapElectra) SizeSSZ() int {
	return h.p.SizeSSZ()
}

func (h *bootstrapElectra) Version() int {
	return version.Electra
}

func (h *bootstrapElectra) Header() interfaces.LightClientHeader {
	return h.header
}

func (h *bootstrapElectra) CurrentSyncCommittee() *pb.SyncCommittee {
	return h.p.CurrentSyncCommittee
}

func (h *bootstrapElectra) CurrentSyncCommitteeBranch() (interfaces.LightClientSyncCommitteeBranch, error) {
	return [5][32]byte{}, consensustypes.ErrNotSupported("CurrentSyncCommitteeBranch", version.Electra)
}

func (h *bootstrapElectra) CurrentSyncCommitteeBranchElectra() (interfaces.LightClientSyncCommitteeBranchElectra, error) {
	return h.currentSyncCommitteeBranch, nil
}
