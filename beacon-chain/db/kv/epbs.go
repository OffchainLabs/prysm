package kv

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
)

func (s *Store) SignedExecutionPayloadHeader(ctx context.Context, blockRoot [32]byte) (interfaces.ROSignedExecutionPayloadHeader, error) {
	b, err := s.Block(ctx, blockRoot)
	if err != nil {
		return nil, err
	}
	if b.IsNil() {
		return nil, ErrNotFound
	}
	return b.Block().Body().SignedExecutionPayloadHeader()
}
