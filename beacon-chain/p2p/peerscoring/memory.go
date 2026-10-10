package peerscoring

import (
	"context"
	"time"
	"unsafe"

	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/dustin/go-humanize"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// Slot sizes of the maps holding scoring state, used to estimate their tables.
const (
	pointerSlotSize = unsafe.Sizeof(peer.ID("")) + unsafe.Sizeof(uintptr(0))
	sliceSlotSize   = unsafe.Sizeof(peer.ID("")) + unsafe.Sizeof([]GossipRejection(nil))
)

// MemoryUsage estimates the heap held by peer scoring state. Strings shared with libp2p
// (topic names, rejection agents) are not counted.
type MemoryUsage struct {
	MeasuredAt time.Time `json:"measured_at"`
	TotalBytes uint64    `json:"total_bytes"`
	// PeerRecordsBytes covers each tracked peer's map slot, ID, record and agent.
	PeerRecordsBytes      uint64 `json:"peer_records_bytes"`
	StrikesBytes          uint64 `json:"strikes_bytes"`
	StatusesBytes         uint64 `json:"statuses_bytes"`
	GossipScoresBytes     uint64 `json:"gossip_scores_bytes"`
	GossipRejectionsBytes uint64 `json:"gossip_rejections_bytes"`
	TrackedPeers          int    `json:"tracked_peers"`
	StrikeEntries         int    `json:"strike_entries"`
	StatusEntries         int    `json:"status_entries"`
	TopicScoreEntries     int    `json:"topic_score_entries"`
	GossipRejectionPeers  int    `json:"gossip_rejection_peers"`
	GossipRejections      int    `json:"gossip_rejections"`
}

// TrackMemoryUsage measures the scoring state at start and then every memoryUsageInterval until
// ctx is canceled, keeping the latest measurement for the debug API and logging it. rejections may be nil.
func (s *Scorer) TrackMemoryUsage(ctx context.Context, rejections *GossipRejectionsStore) {
	ticker := time.NewTicker(s.params.memoryUsageInterval)
	defer ticker.Stop()

	for {
		u := s.measureMemoryUsage(rejections)
		log.WithFields(logrus.Fields{
			"total":            humanize.IBytes(u.TotalBytes),
			"peerRecords":      humanize.IBytes(u.PeerRecordsBytes),
			"strikes":          humanize.IBytes(u.StrikesBytes),
			"statuses":         humanize.IBytes(u.StatusesBytes),
			"gossipScores":     humanize.IBytes(u.GossipScoresBytes),
			"gossipRejections": humanize.IBytes(u.GossipRejectionsBytes),
			"trackedPeers":     u.TrackedPeers,
		}).Info("Peer scoring memory usage")

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// measureMemoryUsage estimates the scorer's and the rejections store's memory, stores the
// result as the latest measurement and returns it. rejections may be nil.
func (s *Scorer) measureMemoryUsage(rejections *GossipRejectionsStore) *MemoryUsage {
	u := &MemoryUsage{MeasuredAt: time.Now().UTC()}
	s.addMemoryUsage(u)
	if rejections != nil {
		rejections.addMemoryUsage(u)
	}
	u.TotalBytes = u.PeerRecordsBytes + u.StrikesBytes + u.StatusesBytes + u.GossipScoresBytes + u.GossipRejectionsBytes
	s.memoryUsage.Store(u)
	return u
}

// addMemoryUsage adds every tracked peer's scoring state to u.
func (s *Scorer) addMemoryUsage(u *MemoryUsage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u.TrackedPeers = len(s.info)
	u.PeerRecordsBytes += mapTableBytes(len(s.info), pointerSlotSize)
	for pid, pi := range s.info {
		u.PeerRecordsBytes += uint64(len(pid)) + uint64(unsafe.Sizeof(PeerScoringInfo{})) + uint64(len(pi.agent))

		u.StrikeEntries += len(pi.strikes)
		u.StrikesBytes += uint64(cap(pi.strikes)) * uint64(unsafe.Sizeof(Strike{}))
		for _, strike := range pi.strikes {
			u.StrikesBytes += uint64(len(strike.Reason))
		}

		if rs := pi.rpcStatus; rs != nil {
			u.StatusEntries++
			u.StatusesBytes += uint64(unsafe.Sizeof(RpcStatus{}))
			if cs := rs.chainState; cs != nil {
				u.StatusesBytes += uint64(unsafe.Sizeof(pb.StatusV2{})) + uint64(cap(cs.ForkDigest)+cap(cs.FinalizedRoot)+cap(cs.HeadRoot))
			}
			if rs.validationError != nil {
				u.StatusesBytes += uint64(len(rs.validationError.Error()))
			}
		}

		u.TopicScoreEntries += len(pi.topicScores)
		u.GossipScoresBytes += mapTableBytes(len(pi.topicScores), pointerSlotSize)
		for _, snap := range pi.topicScores {
			if snap != nil {
				u.GossipScoresBytes += uint64(unsafe.Sizeof(pb.TopicScoreSnapshot{}))
			}
		}
	}
}

func (s *GossipRejectionsStore) addMemoryUsage(u *MemoryUsage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u.GossipRejectionPeers = len(s.rejections)
	u.GossipRejectionsBytes += mapTableBytes(len(s.rejections), sliceSlotSize)
	for pid, entries := range s.rejections {
		u.GossipRejections += len(entries)
		u.GossipRejectionsBytes += uint64(len(pid)) + uint64(cap(entries))*uint64(unsafe.Sizeof(GossipRejection{}))
		for _, rj := range entries {
			u.GossipRejectionsBytes += uint64(len(rj.Reason))
		}
	}
}

// mapTableBytes approximates the table of a Go map with n entries: power-of-two slot groups
// kept at most 7/8 full, each slot carrying one control byte.
func mapTableBytes(n int, slotSize uintptr) uint64 {
	if n == 0 {
		return 0
	}
	slots := uint64(8)
	for n > 8 && slots*7/8 < uint64(n) {
		slots *= 2
	}
	return slots * (uint64(slotSize) + 1)
}
