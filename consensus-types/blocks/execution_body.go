package blocks

import (
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

// executionPayloadBodyV1JSON wraps the V1 JSON body DTO
// (engine_getPayloadBodiesByHashV1).
type executionPayloadBodyV1JSON struct {
	*enginev1.ExecutionPayloadBodyV1
}

var _ interfaces.ExecutionPayloadBody = (*executionPayloadBodyV1JSON)(nil)

// WrappedExecutionPayloadBodyV1JSON wraps the V1 JSON body DTO in the interface.
func WrappedExecutionPayloadBodyV1JSON(p *enginev1.ExecutionPayloadBodyV1) (interfaces.ExecutionPayloadBody, error) {
	w := executionPayloadBodyV1JSON{p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

func (e executionPayloadBodyV1JSON) IsNil() bool {
	return e.ExecutionPayloadBodyV1 == nil
}

func (e executionPayloadBodyV1JSON) Transactions() ([][]byte, error) {
	return enginev1.RecastHexutilByteSlice(e.ExecutionPayloadBodyV1.Transactions), nil
}

func (e executionPayloadBodyV1JSON) Withdrawals() ([]*enginev1.Withdrawal, error) {
	return e.ExecutionPayloadBodyV1.Withdrawals, nil
}

// BlockAccessList is unsupported on the V1 body (pre-Gloas).
func (e executionPayloadBodyV1JSON) BlockAccessList() ([]byte, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// executionPayloadBodyV2JSON wraps the V2 JSON body DTO
// (engine_getPayloadBodiesByHashV2).
type executionPayloadBodyV2JSON struct {
	*enginev1.ExecutionPayloadBodyV2
}

var _ interfaces.ExecutionPayloadBody = (*executionPayloadBodyV2JSON)(nil)

// WrappedExecutionPayloadBodyV2JSON wraps the V2 JSON body DTO in the interface.
func WrappedExecutionPayloadBodyV2JSON(p *enginev1.ExecutionPayloadBodyV2) (interfaces.ExecutionPayloadBody, error) {
	w := executionPayloadBodyV2JSON{p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

func (e executionPayloadBodyV2JSON) IsNil() bool {
	return e.ExecutionPayloadBodyV2 == nil
}

func (e executionPayloadBodyV2JSON) Transactions() ([][]byte, error) {
	return enginev1.RecastHexutilByteSlice(e.ExecutionPayloadBodyV2.Transactions), nil
}

func (e executionPayloadBodyV2JSON) Withdrawals() ([]*enginev1.Withdrawal, error) {
	return e.ExecutionPayloadBodyV2.Withdrawals, nil
}

func (e executionPayloadBodyV2JSON) BlockAccessList() ([]byte, error) {
	if e.ExecutionPayloadBodyV2.BlockAccessList == nil {
		return nil, nil
	}
	return []byte(*e.ExecutionPayloadBodyV2.BlockAccessList), nil
}
