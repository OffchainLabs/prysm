package helpers

import (
	"bytes"
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/pkg/errors"
)

// BlockRootFromState returns the root of the block that produced the given state.
//
// Unlike st.LatestBlockHeader().HashTreeRoot(), a zeroed header state_root is replaced with
// st.HashTreeRoot(ctx) before hashing.
func BlockRootFromState(ctx context.Context, st state.BeaconState) ([32]byte, error) {
	header := st.LatestBlockHeader()
	if header == nil {
		return [32]byte{}, errors.New("state has a nil latest block header")
	}

	zeroHash := params.BeaconConfig().ZeroHash
	if len(header.StateRoot) == 0 || bytes.Equal(header.StateRoot, zeroHash[:]) {
		stateRoot, err := st.HashTreeRoot(ctx)
		if err != nil {
			return [32]byte{}, fmt.Errorf("hash tree root: %w", err)
		}

		header.StateRoot = stateRoot[:]
	}

	root, err := header.HashTreeRoot()
	if err != nil {
		return [32]byte{}, fmt.Errorf("hash tree root: %w", err)
	}

	return root, nil
}
