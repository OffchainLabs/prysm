package sync

import (
	"context"
	"fmt"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// skipcq: SCC-U1000
func (s *Service) syncCommitteeMessageSubscriber(_ context.Context, msg any) error {
	m, ok := msg.(*ethpb.SyncCommitteeMessage)
	if !ok {
		return fmt.Errorf("message was not type *ethpb.SyncCommitteeMessage, type=%T", msg)
	}

	if m == nil {
		return errors.New("nil sync committee message")
	}

	return s.cfg.syncCommsPool.SaveSyncCommitteeMessage(m)
}
