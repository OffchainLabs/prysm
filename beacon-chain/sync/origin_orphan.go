package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var errOrphanedOrigin = errors.New("checkpoint sync origin is not part of the finalized chain, resync required")

const orphanedOriginStreakThreshold = 2

// detectOrphanedOrigin reports when peers finalize a chain that excludes our checkpoint origin.
func (s *Service) detectOrphanedOrigin(ctx context.Context) {
	cp := s.cfg.chain.FinalizedCheckpt()
	if cp == nil || s.orphanedOriginStreak == nil {
		return
	}
	if _, err := s.cfg.beaconDB.OriginCheckpointBlockRoot(ctx); err != nil {
		return
	}
	ours := bytesutil.ToBytes32(cp.Root)
	conflicting := 0
	for _, id := range s.cfg.p2p.Peers().Connected() {
		cs, err := s.cfg.p2p.Peers().ChainState(id)
		if err != nil || cs == nil || cs.FinalizedEpoch < cp.Epoch {
			continue
		}
		theirs := bytesutil.ToBytes32(cs.FinalizedRoot)
		if theirs == ours || s.cfg.beaconDB.HasBlock(ctx, theirs) {
			continue
		}
		conflicting++
	}

	if conflicting == 0 || conflicting < flags.Get().MinimumSyncPeers {
		s.orphanedOriginStreak.Store(0)
		originOrphanedSuspected.Set(0)
		return
	}

	if s.orphanedOriginStreak.Add(1) < orphanedOriginStreakThreshold {
		return
	}

	originOrphanedSuspected.Set(1)
	log.WithFields(logrus.Fields{
		"finalizedEpoch": cp.Epoch,
		"finalizedRoot":  fmt.Sprintf("%#x", cp.Root),
		"peers":          conflicting,
	}).Error("Peers have finalized a chain that does not contain our checkpoint sync origin. This node cannot recover; delete the database and resync from a new checkpoint")
}
