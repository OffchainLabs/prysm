package testutil

import (
	"context"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/options"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
)

// MockBlocker is a fake implementation of lookup.Blocker.
type MockBlocker struct {
	BlockToReturn            interfaces.ReadOnlySignedBeaconBlock
	ErrorToReturn            error
	SlotBlockMap             map[primitives.Slot]interfaces.ReadOnlySignedBeaconBlock
	RootBlockMap             map[[32]byte]interfaces.ReadOnlySignedBeaconBlock
	DataColumnsFunc          func(ctx context.Context, id string, indices []int) ([]blocks.VerifiedRODataColumn, *core.RpcError)
	DataColumnsToReturn      []blocks.VerifiedRODataColumn
	DataColumnsErrorToReturn *core.RpcError
}

// Block --
func (m *MockBlocker) Block(_ context.Context, b []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	if m.ErrorToReturn != nil {
		return nil, m.ErrorToReturn
	}
	if m.BlockToReturn != nil {
		return m.BlockToReturn, nil
	}
	slotNumber, parseErr := strconv.ParseUint(string(b), 10, 64)
	if parseErr != nil {
		//nolint:nilerr
		return m.RootBlockMap[bytesutil.ToBytes32(b)], nil
	}
	return m.SlotBlockMap[primitives.Slot(slotNumber)], nil
}

// Blobs --
func (*MockBlocker) Blobs(_ context.Context, _ string, _ ...options.BlobsOption) ([]*blocks.VerifiedROBlob, *core.RpcError) {
	return nil, &core.RpcError{}
}

// DataColumns --
func (m *MockBlocker) DataColumns(ctx context.Context, id string, indices []int) ([]blocks.VerifiedRODataColumn, *core.RpcError) {
	if m.DataColumnsFunc != nil {
		return m.DataColumnsFunc(ctx, id, indices)
	}
	if m.DataColumnsErrorToReturn != nil {
		return nil, m.DataColumnsErrorToReturn
	}
	if m.DataColumnsToReturn != nil {
		return m.DataColumnsToReturn, nil
	}
	return nil, &core.RpcError{}
}
