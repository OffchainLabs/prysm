package blockprovider_test

import (
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/blockprovider"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/time"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)

	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{
		BlockBatchLimit:            64,
		BlockBatchLimitBurstFactor: 10,
	})
	defer func() {
		flags.Init(resetFlags)
	}()
	os.Exit(m.Run())
}

// roundScore returns score rounded in accordance with the selector's rounding factor.
func roundScore(score float64) float64 {
	return math.Round(score*blockprovider.ScoreRoundingFactor) / blockprovider.ScoreRoundingFactor
}

func TestSelector_Score(t *testing.T) {
	batchSize := uint64(flags.Get().BlockBatchLimit)
	tests := []struct {
		name   string
		update func(selector *blockprovider.Selector)
		check  func(selector *blockprovider.Selector)
	}{
		{
			name: "nonexistent peer",
			check: func(selector *blockprovider.Selector) {
				assert.Equal(t, selector.MaxScore(), selector.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "existent peer with zero score",
			update: func(selector *blockprovider.Selector) {
				selector.Touch("peer1")
			},
			check: func(selector *blockprovider.Selector) {
				assert.Equal(t, 0.0, selector.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "existent peer via increment",
			update: func(selector *blockprovider.Selector) {
				selector.IncrementProcessedBlocks("peer1", 0)
			},
			check: func(selector *blockprovider.Selector) {
				assert.Equal(t, 0.0, selector.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "boost score of stale peer",
			update: func(selector *blockprovider.Selector) {
				batchWeight := selector.Params().ProcessedBatchWeight
				selector.IncrementProcessedBlocks("peer1", batchSize*3)
				assert.Equal(t, roundScore(batchWeight*3), selector.Score("peer1"), "Unexpected score")
				selector.Touch("peer1", time.Now().Add(-1*selector.Params().StalePeerRefreshInterval))
			},
			check: func(selector *blockprovider.Selector) {
				assert.Equal(t, selector.MaxScore(), selector.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "increment with 0 score",
			update: func(selector *blockprovider.Selector) {
				// Increment to zero (provider is added to cache but score is unchanged).
				selector.IncrementProcessedBlocks("peer1", 0)
			},
			check: func(selector *blockprovider.Selector) {
				assert.Equal(t, 0.0, selector.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "partial score",
			update: func(selector *blockprovider.Selector) {
				// Partial score (less than a single batch of blocks processed).
				selector.IncrementProcessedBlocks("peer1", batchSize/2)
			},
			check: func(selector *blockprovider.Selector) {
				assert.Equal(t, 0.0, selector.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "single batch",
			update: func(selector *blockprovider.Selector) {
				selector.IncrementProcessedBlocks("peer1", batchSize)
			},
			check: func(selector *blockprovider.Selector) {
				batchWeight := selector.Params().ProcessedBatchWeight
				assert.Equal(t, roundScore(batchWeight), selector.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "multiple batches",
			update: func(selector *blockprovider.Selector) {
				selector.IncrementProcessedBlocks("peer1", batchSize*7)
			},
			check: func(selector *blockprovider.Selector) {
				batchWeight := selector.Params().ProcessedBatchWeight
				assert.Equal(t, roundScore(batchWeight*7), selector.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "maximum score cap",
			update: func(selector *blockprovider.Selector) {
				batchWeight := selector.Params().ProcessedBatchWeight
				selector.IncrementProcessedBlocks("peer1", batchSize*2)
				assert.Equal(t, roundScore(batchWeight*2), selector.Score("peer1"), "Unexpected score")
				selector.IncrementProcessedBlocks("peer1", selector.Params().ProcessedBlocksCap)
			},
			check: func(selector *blockprovider.Selector) {
				assert.Equal(t, selector.Params().ProcessedBlocksCap, selector.ProcessedBlocks("peer1"))
				assert.Equal(t, 1.0, selector.Score("peer1"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := blockprovider.NewSelector(t.Context(), &blockprovider.SelectorConfig{})
			if tt.update != nil {
				tt.update(selector)
			}
			tt.check(selector)
		})
	}
}

func TestSelector_GettersSetters(t *testing.T) {
	selector := blockprovider.NewSelector(t.Context(), nil)

	assert.Equal(t, uint64(0), selector.ProcessedBlocks("peer1"), "Unexpected count for unregistered peer")
	selector.IncrementProcessedBlocks("peer1", 64)
	assert.Equal(t, uint64(64), selector.ProcessedBlocks("peer1"))
}

func TestSelector_WeightSorted(t *testing.T) {
	selector := blockprovider.NewSelector(t.Context(), &blockprovider.SelectorConfig{
		ProcessedBatchWeight: 0.01,
	})
	batchSize := uint64(flags.Get().BlockBatchLimit)
	r := rand.NewDeterministicGenerator()

	reverse := func(pids []peer.ID) []peer.ID {
		tmp := make([]peer.ID, len(pids))
		copy(tmp, pids)
		for i, j := 0, len(tmp)-1; i < j; i, j = i+1, j-1 {
			tmp[i], tmp[j] = tmp[j], tmp[i]
		}
		return tmp
	}

	shuffle := func(pids []peer.ID) []peer.ID {
		tmp := make([]peer.ID, len(pids))
		copy(tmp, pids)
		r.Shuffle(len(tmp), func(i, j int) {
			tmp[i], tmp[j] = tmp[j], tmp[i]
		})
		return tmp
	}

	var pids []peer.ID
	for i := range uint64(10) {
		pid := peer.ID(strconv.FormatUint(i, 10))
		selector.IncrementProcessedBlocks(pid, i*batchSize)
		pids = append(pids, pid)
	}
	// Make sure that peers scores are correct (peer(n).score > peer(n-1).score).
	// Peers should be returned in descending order (by score).
	assert.DeepEqual(t, reverse(pids), selector.Sorted(pids, nil))

	// Run weighted sort lots of time, to get accurate statistics of whether more heavy items
	// are indeed preferred when sorting.
	scores := make(map[peer.ID]int, len(pids))
	for range 1000 {
		score := len(pids) - 1
		// The earlier in the list the item is, the more of a score will it get.
		for _, pid := range selector.WeightSorted(r, shuffle(pids), nil) {
			scores[pid] += score
			score--
		}
	}
	var scoredPIDs []peer.ID
	for pid := range scores {
		scoredPIDs = append(scoredPIDs, pid)
	}
	sort.Slice(scoredPIDs, func(i, j int) bool {
		return scores[scoredPIDs[i]] > scores[scoredPIDs[j]]
	})
	assert.Equal(t, len(pids), len(scoredPIDs))
	assert.DeepEqual(t, reverse(pids), scoredPIDs, "Expected items with more weight to be picked more often")
}

func TestSelector_Sorted(t *testing.T) {
	batchSize := uint64(flags.Get().BlockBatchLimit)
	tests := []struct {
		name   string
		update func(s *blockprovider.Selector)
		score  func(pid peer.ID, score float64) float64
		have   []peer.ID
		want   []peer.ID
	}{
		{
			name:   "no peers",
			update: func(*blockprovider.Selector) {},
			have:   []peer.ID{},
			want:   []peer.ID{},
		},
		{
			name: "same scores",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", 16)
				s.IncrementProcessedBlocks("peer2", 16)
				s.IncrementProcessedBlocks("peer3", 16)
			},
			have: []peer.ID{"peer1", "peer2", "peer3"},
			want: []peer.ID{"peer1", "peer2", "peer3"},
		},
		{
			name: "same scores multiple batches",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", batchSize*7+16)
				s.IncrementProcessedBlocks("peer2", batchSize*7+16)
				s.IncrementProcessedBlocks("peer3", batchSize*7+16)
			},
			have: []peer.ID{"peer1", "peer2", "peer3"},
			want: []peer.ID{"peer1", "peer2", "peer3"},
		},
		{
			name: "same scores multiple batches unequal blocks",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", batchSize*7+6)
				s.IncrementProcessedBlocks("peer2", batchSize*7+16)
				s.IncrementProcessedBlocks("peer3", batchSize*7+26)
			},
			have: []peer.ID{"peer1", "peer2", "peer3"},
			want: []peer.ID{"peer1", "peer2", "peer3"},
		},
		{
			name: "different scores",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", batchSize*3)
				s.IncrementProcessedBlocks("peer2", batchSize*1)
				s.IncrementProcessedBlocks("peer3", batchSize*2)
			},
			have: []peer.ID{"peer3", "peer2", "peer1"},
			want: []peer.ID{"peer1", "peer3", "peer2"},
		},
		{
			name: "custom scorer",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", batchSize*3)
				s.IncrementProcessedBlocks("peer2", batchSize*1)
				s.IncrementProcessedBlocks("peer3", batchSize*2)
			},
			score: func(pid peer.ID, score float64) float64 {
				if pid == "peer2" {
					return score + 0.3 // 0.2 + 0.3 = 0.5 > 0.4 (of peer3)
				}
				if pid == "peer1" {
					return 0.0
				}
				return score
			},
			have: []peer.ID{"peer3", "peer2", "peer1"},
			want: []peer.ID{"peer2", "peer3", "peer1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := blockprovider.NewSelector(t.Context(), &blockprovider.SelectorConfig{
				ProcessedBatchWeight: 0.2,
			})
			tt.update(selector)
			assert.DeepEqual(t, tt.want, selector.Sorted(tt.have, tt.score))
		})
	}
}

func TestSelector_MaxScore(t *testing.T) {
	batchSize := uint64(flags.Get().BlockBatchLimit)

	tests := []struct {
		name string
		cfg  *blockprovider.SelectorConfig
		want float64
	}{
		{
			name: "default config",
			cfg:  &blockprovider.SelectorConfig{},
			want: 1.0,
		},
		{
			name: "custom config",
			cfg: &blockprovider.SelectorConfig{
				ProcessedBatchWeight: 0.5,
				ProcessedBlocksCap:   batchSize * 300,
			},
			want: 150.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := blockprovider.NewSelector(t.Context(), tt.cfg)
			assert.Equal(t, tt.want, selector.MaxScore())
		})
	}
}

func TestSelector_FormatScorePretty(t *testing.T) {
	batchSize := uint64(flags.Get().BlockBatchLimit)
	format := "[%0.1f%%, raw: %0.2f,  blocks: %d/1280]"

	tests := []struct {
		name   string
		update func(s *blockprovider.Selector)
		check  func(s *blockprovider.Selector)
	}{
		{
			name:   "peer not registered",
			update: nil,
			check: func(s *blockprovider.Selector) {
				assert.Equal(t, fmt.Sprintf(format, 100.0, 1.0, 0), s.FormatScorePretty("peer1"))
			},
		},
		{
			name: "peer registered zero blocks",
			update: func(s *blockprovider.Selector) {
				s.Touch("peer1")
			},
			check: func(s *blockprovider.Selector) {
				assert.Equal(t, fmt.Sprintf(format, 0.0, 0.0, 0), s.FormatScorePretty("peer1"))
			},
		},
		{
			name: "partial batch",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", batchSize/4)
			},
			check: func(s *blockprovider.Selector) {
				assert.Equal(t, fmt.Sprintf(format, 0.0, 0.0, batchSize/4), s.FormatScorePretty("peer1"))
			},
		},
		{
			name: "single batch",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", batchSize)
			},
			check: func(s *blockprovider.Selector) {
				assert.Equal(t, fmt.Sprintf(format, 5.0, 0.05, batchSize), s.FormatScorePretty("peer1"))
			},
		},
		{
			name: "3/2 of a batch",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", batchSize*3/2)
			},
			check: func(s *blockprovider.Selector) {
				assert.Equal(t, fmt.Sprintf(format, 5.0, 0.05, batchSize*3/2), s.FormatScorePretty("peer1"))
			},
		},
		{
			name: "multiple batches",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", batchSize*5)
			},
			check: func(s *blockprovider.Selector) {
				assert.Equal(t, fmt.Sprintf(format, 25.0, 0.05*5, batchSize*5), s.FormatScorePretty("peer1"))
			},
		},
		{
			name: "multiple batches max score",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", s.Params().ProcessedBlocksCap*5)
			},
			check: func(s *blockprovider.Selector) {
				want := fmt.Sprintf(format, 100.0, 1.0, s.Params().ProcessedBlocksCap)
				assert.Equal(t, want, s.FormatScorePretty("peer1"))
			},
		},
		{
			name: "decaying",
			update: func(s *blockprovider.Selector) {
				s.IncrementProcessedBlocks("peer1", batchSize*5)
				s.IncrementProcessedBlocks("peer1", batchSize)
				s.IncrementProcessedBlocks("peer1", batchSize/4)
				want := fmt.Sprintf(format, 30.0, 0.05*6, batchSize*6+batchSize/4)
				assert.Equal(t, want, s.FormatScorePretty("peer1"))
				// Maximize block count.
				s.IncrementProcessedBlocks("peer1", s.Params().ProcessedBlocksCap)
				want = fmt.Sprintf(format, 100.0, 1.0, s.Params().ProcessedBlocksCap)
				assert.Equal(t, want, s.FormatScorePretty("peer1"))
				// Half of blocks is to be decayed.
				s.Decay()
			},
			check: func(s *blockprovider.Selector) {
				want := fmt.Sprintf(format, 50.0, 0.5, s.Params().ProcessedBlocksCap/2)
				assert.Equal(t, want, s.FormatScorePretty("peer1"))
			},
		},
	}

	selectorGen := func(t *testing.T) *blockprovider.Selector {
		return blockprovider.NewSelector(t.Context(), &blockprovider.SelectorConfig{
			ProcessedBatchWeight: 0.05,
			ProcessedBlocksCap:   20 * batchSize,
			Decay:                10 * batchSize,
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := selectorGen(t)
			if tt.update != nil {
				tt.update(selector)
			}
			tt.check(selector)
		})
	}
}
