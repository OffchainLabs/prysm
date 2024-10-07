package testing

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/das"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
)

// ReceiveExecutionPayloadEnvelope mocks the  method in chain service.
func (s *ChainService) ReceiveExecutionPayloadEnvelope(ctx context.Context, env interfaces.ROExecutionPayloadEnvelope, _ das.AvailabilityStore) error {
	if s.ReceiveBlockMockErr != nil {
		return s.ReceiveBlockMockErr
	}
	if s.State == nil {
		return ErrNilState
	}
	if s.State.Slot() == env.Slot() {
		if err := s.State.SetLatestFullSlot(s.State.Slot()); err != nil {
			return err
		}
	}
	s.ExecutionPayloadEnvelope = env
	return nil
}
