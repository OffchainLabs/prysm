package sync

import (
	"slices"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

type columnRankedPeer struct {
	peerID    peer.ID
	nodeID    enode.ID
	custodied []uint64
	cov       float64
}

func (p *columnRankedPeer) covered(needed peerdas.ColumnIndices) []uint64 {
	covered := make([]uint64, 0, len(p.custodied))
	for col, want := range needed {
		if want && p.custodied[col] == 1 {
			covered = append(covered, uint64(col))
		}
	}
	return covered
}

func (p *columnRankedPeer) coverageScore(rarity []float64) float64 {
	if p.cov == 0 {
		p.cov = coverageScore(p.custodied, rarity)
	}
	return p.cov
}

type ColumnPeerRank struct {
	peers        map[peer.ID]*columnRankedPeer
	freq         []colFreq
	rarity       []float64
	rg           rand.Rand
	covScoreRank []*columnRankedPeer
}

func coverageScore(covered []uint64, rarity []float64) float64 {
	score := 0.0
	for _, col := range covered {
		if col >= uint64(len(rarity)) {
			continue
		}
		score += rarity[col]
	}
	return score
}

func (m *ColumnPeerRank) HighestForIndices(needed peerdas.ColumnIndices, busy map[peer.ID]bool) (peer.ID, []uint64, error) {
	// - find the custodied column with the lowest frequency
	// - collect all the peers that have custody of that column
	// - score the peers by how many other of the needed columns they ave
	// -- or, score them by the rank of the columns they have??
	for _, cf := range m.freq {
		if !needed[cf.col] {
			continue
		}
		if cf.freq() == 0 {
			continue
		}
		var best *columnRankedPeer
		bestScore, bestCoverage := 0.0, make([]uint64, 1)
		for _, p := range cf.custodians {
			if busy[p.peerID] {
				continue
			}
			coverage := p.covered(needed)
			if len(coverage) == 0 {
				continue
			}
			pscore := coverageScore(coverage, m.rarity)
			if pscore > bestScore {
				best, bestScore, bestCoverage = p, pscore, coverage
			}
		}
		if best != nil {
			return best.peerID, bestCoverage, nil
		}
	}

	return "", nil, errors.New("no peers able to cover needed columns")
}

func NeededCoveredIntersection(needed peerdas.ColumnIndices, covered []uint64) []uint64 {
	intersection := make([]uint64, 0, len(covered))
	for _, col := range covered {
		if needed[col] {
			intersection = append(intersection, col)
		}
	}
	return intersection
}

// Lowest returns the lowest scoring peer in the set. This can be used to pick a peer
// for block requests, preserving the peers that have the highest coverage scores
// for column requests.
func (m *ColumnPeerRank) Lowest(busy map[peer.ID]bool) (peer.ID, error) {
	for i := len(m.covScoreRank) - 1; i >= 0; i-- {
		p := m.covScoreRank[i]
		if !busy[p.peerID] {
			return p.peerID, nil
		}
	}
	return "", errors.New("no peers available")
}

type colFreq struct {
	col        uint64
	custodians []*columnRankedPeer
}

func (f colFreq) rarity() float64 {
	if f.freq() == 0 {
		return 1
	}
	return 1 / float64(f.freq())
}

func (f colFreq) freq() int {
	return len(f.custodians)
}

type colFreqs []colFreq

func (s colFreqs) rarity() []float64 {
	ra := make([]float64, len(s))
	for _, f := range s {
		ra[f.col] = f.rarity()
	}
	return ra
}

// ColumnMatrix computes a grid of column custody x peer.
func ComputeColumnPeerRank(peers []peer.ID, p2pSvc p2p.P2P) (*ColumnPeerRank, error) {
	nc := params.BeaconConfig().NumberOfColumns
	grid := make(map[peer.ID]*columnRankedPeer, len(peers))
	freqByColumn := make([]colFreq, nc)
	for i := range freqByColumn {
		freqByColumn[i].col = uint64(i)
	}
	for _, peer := range peers {
		nodeID, err := p2p.ConvertPeerIDToNodeID(peer)
		if err != nil {
			log.WithField("peerID", peer).WithError(err).Debug("Failed to convert peer ID to node ID.")
			continue
		}
		dasInfo, _, err := peerdas.Info(nodeID, p2pSvc.CustodyGroupCountFromPeer(peer))
		if err != nil {
			log.WithField("peerID", peer).WithField("nodeID", nodeID).WithError(err).Debug("Failed to derive custody groups from peer.")
			return nil, errors.Wrap(err, "custody groups")
		}
		p := &columnRankedPeer{
			peerID:    peer,
			nodeID:    nodeID,
			custodied: make([]uint64, nc),
		}
		for c, v := range dasInfo.CustodyColumns {
			if c > nc-1 {
				return nil, errors.Errorf("column %d is out of bounds", c)
			}
			if v {
				p.custodied[c] = 1
				freqByColumn[c].custodians = append(freqByColumn[c].custodians, p)
			}
		}
		grid[peer] = p
	}

	var colByFreq colFreqs
	colByFreq = slices.SortedFunc(slices.Values(freqByColumn), func(a, b colFreq) int {
		if a.freq() == b.freq() {
			return 0
		}
		if a.freq() < b.freq() {
			return -1
		}
		return 1
	})
	rarity := colByFreq.rarity()

	covScoreRank := make([]*columnRankedPeer, 0, len(grid))
	for _, p := range grid {
		covScoreRank = append(covScoreRank, p)
	}
	slices.SortFunc(covScoreRank, func(a, b *columnRankedPeer) int {
		if a.coverageScore(rarity) == b.coverageScore(rarity) {
			return 0
		}
		if a.coverageScore(rarity) < b.coverageScore(rarity) {
			return -1
		}
		return 1
	})

	return &ColumnPeerRank{
		peers:        grid,
		freq:         colByFreq,
		rg:           *rand.NewGenerator(),
		rarity:       rarity,
		covScoreRank: covScoreRank,
	}, nil
}
