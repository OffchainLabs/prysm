package testing

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
)

// ReceiveExecutionPayloadEnvelope mocks the  method in chain service.
func (s *ChainService) ReceiveExecutionPayloadEnvelope(ctx context.Context, env interfaces.ROSignedExecutionPayloadEnvelope, _ das.AvailabilityStore) error {
	if s.ReceiveBlockMockErr != nil {
		return s.ReceiveBlockMockErr
	}
	if s.State == nil {
		return ErrNilState
	}
	e, err := env.Envelope()
	if err != nil {
		return err
	}

	if s.State.Slot() == e.Slot() {
		if err := s.State.SetLatestFullSlot(s.State.Slot()); err != nil {
			return err
		}
	}
	s.ExecutionPayloadEnvelope = e
	return nil
}
