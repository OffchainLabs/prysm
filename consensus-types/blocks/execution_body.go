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
