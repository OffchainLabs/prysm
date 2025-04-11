package sync

import (
	"context"
	"fmt"

	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// skipcq: SCC-U1000
func (s *Service) syncCommitteeMessageSubscriber(_ context.Context, msg proto.Message) error {
	m, ok := msg.(*ethpb.SyncCommitteeMessage)
	if !ok {
		return fmt.Errorf("message was not type *ethpb.SyncCommitteeMessage, type=%T", msg)
	}

	if m == nil {
		return errors.New("nil sync committee message")
	}

	return s.cfg.syncCommsPool.SaveSyncCommitteeMessage(m)
}
