package blocks

import (
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
)

// executionPayloadBodyFulu wraps the Osaka/Fulu Engine API v2 payload body.
// It has no block access list.
type executionPayloadBodyFulu struct {
	*enginev2.ExecutionPayloadBodyFulu
}

// WrappedExecutionPayloadBodyFulu wraps the proto in the fork-generic interface.
func WrappedExecutionPayloadBodyFulu(p *enginev2.ExecutionPayloadBodyFulu) (interfaces.ExecutionPayloadBody, error) {
	w := executionPayloadBodyFulu{p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

func (e executionPayloadBodyFulu) IsNil() bool {
	return e.ExecutionPayloadBodyFulu == nil
}

func (e executionPayloadBodyFulu) Transactions() ([][]byte, error) {
	return e.GetTransactions(), nil
}

func (e executionPayloadBodyFulu) Withdrawals() ([]*enginev1.Withdrawal, error) {
	return e.GetWithdrawals(), nil
}

// BlockAccessList is unsupported before Gloas/amsterdam.
func (e executionPayloadBodyFulu) BlockAccessList() ([]byte, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// executionPayloadBodyGloas wraps the Amsterdam/Gloas Engine API v2 payload body,
// which additionally carries the block access list.
type executionPayloadBodyGloas struct {
	*enginev2.ExecutionPayloadBodyGloas
}

// WrappedExecutionPayloadBodyGloas wraps the proto in the fork-generic interface.
func WrappedExecutionPayloadBodyGloas(p *enginev2.ExecutionPayloadBodyGloas) (interfaces.ExecutionPayloadBody, error) {
	w := executionPayloadBodyGloas{p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

func (e executionPayloadBodyGloas) IsNil() bool {
	return e.ExecutionPayloadBodyGloas == nil
}

func (e executionPayloadBodyGloas) Transactions() ([][]byte, error) {
	return e.GetTransactions(), nil
}

func (e executionPayloadBodyGloas) Withdrawals() ([]*enginev1.Withdrawal, error) {
	return e.GetWithdrawals(), nil
}

func (e executionPayloadBodyGloas) BlockAccessList() ([]byte, error) {
	return e.GetBlockAccessList(), nil
}

// The proto/engine/v1 ExecutionPayloadBody{,V2} are the JSON-RPC (de)serialization
// DTOs (no SSZ codec), distinct from the SSZ proto/engine/v2 bodies above. The
// JSON transport wraps them so it returns the same fork-generic interface.

// executionPayloadBodyV1JSON wraps the V1 JSON body DTO
// (engine_getPayloadBodiesByHashV1); it has no block access list.
type executionPayloadBodyV1JSON struct {
	*enginev1.ExecutionPayloadBodyV1
}

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
// (engine_getPayloadBodiesByHashV2), which carries the block access list.
type executionPayloadBodyV2JSON struct {
	*enginev1.ExecutionPayloadBodyV2
}

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
