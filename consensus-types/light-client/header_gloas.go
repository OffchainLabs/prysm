package light_client

import (
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	consensustypes "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"google.golang.org/protobuf/proto"
)

type headerGloas struct {
	p               *pb.LightClientHeaderGloas
	executionBranch interfaces.LightClientExecutionBranchGloas
}

var _ interfaces.LightClientHeader = &headerGloas{}

func NewWrappedHeaderGloas(p *pb.LightClientHeaderGloas) (interfaces.LightClientHeader, error) {
	if p == nil || p.Beacon == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	if len(p.ExecutionBlockHash) != fieldparams.RootLength {
		return nil, fmt.Errorf("execution block hash has length %d instead of expected %d", len(p.ExecutionBlockHash), fieldparams.RootLength)
	}
	branch, err := createBranch[interfaces.LightClientExecutionBranchGloas]("execution", p.ExecutionBranch, fieldparams.ExecutionBranchDepthGloas)
	if err != nil {
		return nil, err
	}
	return &headerGloas{p: p, executionBranch: branch}, nil
}

func (h *headerGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	return h.p.MarshalSSZTo(dst)
}

func (h *headerGloas) MarshalSSZ() ([]byte, error) {
	return h.p.MarshalSSZ()
}

func (h *headerGloas) SizeSSZ() int {
	return h.p.SizeSSZ()
}

func (h *headerGloas) Proto() proto.Message {
	return h.p
}

func (h *headerGloas) Version() int {
	return version.Gloas
}

func (h *headerGloas) Beacon() *pb.BeaconBlockHeader {
	return h.p.Beacon
}

func (h *headerGloas) ExecutionBlockHash() ([fieldparams.RootLength]byte, error) {
	return bytesutil.ToBytes32(h.p.ExecutionBlockHash), nil
}

func (h *headerGloas) ExecutionBranchGloas() (interfaces.LightClientExecutionBranchGloas, error) {
	return h.executionBranch, nil
}

func (h *headerGloas) Execution() (interfaces.ExecutionData, error) {
	return nil, consensustypes.ErrNotSupported("Execution", h.Version())
}

func (h *headerGloas) ExecutionBranch() (interfaces.LightClientExecutionBranch, error) {
	return interfaces.LightClientExecutionBranch{}, consensustypes.ErrNotSupported("ExecutionBranch", h.Version())
}
