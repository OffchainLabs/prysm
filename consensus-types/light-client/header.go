package light_client

import (
	"fmt"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	consensustypes "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"google.golang.org/protobuf/proto"
)

func NewWrappedHeader(m proto.Message) (interfaces.LightClientHeader, error) {
	if m == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	switch t := m.(type) {
	case *pb.LightClientHeaderAltair:
		return NewWrappedHeaderAltair(t)
	case *pb.LightClientHeaderCapella:
		return NewWrappedHeaderCapella(t)
	case *pb.LightClientHeaderDeneb:
		return NewWrappedHeaderDeneb(t)
	default:
		return nil, fmt.Errorf("cannot construct light client header from type %T", t)
	}
}

type headerAltair struct {
	p *pb.LightClientHeaderAltair
}

var _ interfaces.LightClientHeader = &headerAltair{}

func NewWrappedHeaderAltair(p *pb.LightClientHeaderAltair) (interfaces.LightClientHeader, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	return &headerAltair{p: p}, nil
}

func (h *headerAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	return h.p.MarshalSSZTo(dst)
}

func (h *headerAltair) MarshalSSZ() ([]byte, error) {
	return h.p.MarshalSSZ()
}

func (h *headerAltair) SizeSSZ() int {
	return h.p.SizeSSZ()
}

func (h *headerAltair) Proto() proto.Message {
	return h.p
}

func (h *headerAltair) Version() int {
	return version.Altair
}

func (h *headerAltair) Beacon() *pb.BeaconBlockHeader {
	return h.p.Beacon
}

func (h *headerAltair) Execution() (interfaces.ExecutionData, error) {
	return nil, consensustypes.ErrNotSupported("Execution", version.Altair)
}

func (h *headerAltair) ExecutionBranch() (interfaces.LightClientExecutionBranch, error) {
	return interfaces.LightClientExecutionBranch{}, consensustypes.ErrNotSupported("ExecutionBranch", version.Altair)
}

type headerCapella struct {
	p               *pb.LightClientHeaderCapella
	execution       interfaces.ExecutionData
	executionBranch interfaces.LightClientExecutionBranch
}

var _ interfaces.LightClientHeader = &headerCapella{}

func NewWrappedHeaderCapella(p *pb.LightClientHeaderCapella) (interfaces.LightClientHeader, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	execution, err := blocks.WrappedExecutionPayloadHeaderCapella(p.Execution)
	if err != nil {
		return nil, err
	}

	branch, err := createBranch[interfaces.LightClientExecutionBranch](
		"execution",
		p.ExecutionBranch,
		fieldparams.ExecutionBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &headerCapella{
		p:               p,
		execution:       execution,
		executionBranch: branch,
	}, nil
}

func (h *headerCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	return h.p.MarshalSSZTo(dst)
}

func (h *headerCapella) MarshalSSZ() ([]byte, error) {
	return h.p.MarshalSSZ()
}

func (h *headerCapella) SizeSSZ() int {
	return h.p.SizeSSZ()
}

func (h *headerCapella) Proto() proto.Message {
	return h.p
}

func (h *headerCapella) Version() int {
	return version.Capella
}

func (h *headerCapella) Beacon() *pb.BeaconBlockHeader {
	return h.p.Beacon
}

func (h *headerCapella) Execution() (interfaces.ExecutionData, error) {
	return h.execution, nil
}

func (h *headerCapella) ExecutionBranch() (interfaces.LightClientExecutionBranch, error) {
	return h.executionBranch, nil
}

type headerDeneb struct {
	p               *pb.LightClientHeaderDeneb
	execution       interfaces.ExecutionData
	executionBranch interfaces.LightClientExecutionBranch
}

var _ interfaces.LightClientHeader = &headerDeneb{}

func NewWrappedHeaderDeneb(p *pb.LightClientHeaderDeneb) (interfaces.LightClientHeader, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	execution, err := blocks.WrappedExecutionPayloadHeaderDeneb(p.Execution)
	if err != nil {
		return nil, err
	}

	branch, err := createBranch[interfaces.LightClientExecutionBranch](
		"execution",
		p.ExecutionBranch,
		fieldparams.ExecutionBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &headerDeneb{
		p:               p,
		execution:       execution,
		executionBranch: branch,
	}, nil
}

func (h *headerDeneb) MarshalSSZTo(dst []byte) ([]byte, error) {
	return h.p.MarshalSSZTo(dst)
}

func (h *headerDeneb) MarshalSSZ() ([]byte, error) {
	return h.p.MarshalSSZ()
}

func (h *headerDeneb) SizeSSZ() int {
	return h.p.SizeSSZ()
}

func (h *headerDeneb) Proto() proto.Message {
	return h.p
}

func (h *headerDeneb) Version() int {
	return version.Deneb
}

func (h *headerDeneb) Beacon() *pb.BeaconBlockHeader {
	return h.p.Beacon
}

func (h *headerDeneb) Execution() (interfaces.ExecutionData, error) {
	return h.execution, nil
}

func (h *headerDeneb) ExecutionBranch() (interfaces.LightClientExecutionBranch, error) {
	return h.executionBranch, nil
}
