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

// ColumnScarcityRanking is a structure that maps out the intersection of peer custody and column indices
// to weight each peer based on the scarcity of the columns they custody. This allows us to prioritize
// requests for more scarce columns to peers that custody them, so that we don't waste our bandwidth allocation
// making requests for more common columns from peers that can provide the more scarce columns.
type ColumnScarcityRanking struct {
	peers        map[peer.ID]*peerColumnCoverage
	freq         []colFreq
	scarcity     []float64
	rg           rand.Rand
	covScoreRank []*peerColumnCoverage
}

// NewColumnScarcityRanking computes the ColumnScarcityRanking based on the current view of columns custodied
// by the given set of peers.
func NewColumnScarcityRanking(peers []peer.ID, p2pSvc p2p.P2P) (*ColumnScarcityRanking, error) {
	nc := params.BeaconConfig().NumberOfColumns
	rankingsByPeer := make(map[peer.ID]*peerColumnCoverage, len(peers))
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
		p := &peerColumnCoverage{
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
		rankingsByPeer[peer] = p
	}

	colByFreq := slices.SortedFunc(slices.Values(freqByColumn), func(a, b colFreq) int {
		if a.freq() == b.freq() {
			return 0
		}
		if a.freq() < b.freq() {
			return -1
		}
		return 1
	})

	scarcity := columnScarcity(colByFreq)
	covScoreRank := make([]*peerColumnCoverage, 0, len(rankingsByPeer))
	for _, p := range rankingsByPeer {
		covScoreRank = append(covScoreRank, p)
	}
	slices.SortFunc(covScoreRank, func(a, b *peerColumnCoverage) int {
		if a.score(scarcity) == b.score(scarcity) {
			return 0
		}
		if a.score(scarcity) < b.score(scarcity) {
			return -1
		}
		return 1
	})

	return &ColumnScarcityRanking{
		peers:        rankingsByPeer,
		freq:         colByFreq,
		rg:           *rand.NewGenerator(),
		scarcity:     scarcity,
		covScoreRank: covScoreRank,
	}, nil
}

// ForColumns returns the best peer to request columns from, based on the scarcity of the columns needed.
func (m *ColumnScarcityRanking) ForColumns(needed peerdas.ColumnIndices, busy map[peer.ID]bool) (peer.ID, []uint64, error) {
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
		var best *peerColumnCoverage
		bestScore, bestCoverage := 0.0, make([]uint64, 1)
		for _, p := range cf.custodians {
			if busy[p.peerID] {
				continue
			}
			coverage := p.covered(needed)
			if len(coverage) == 0 {
				continue
			}
			pscore := coverageScore(coverage, m.scarcity)
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

// ForBlocks returns the lowest scoring peer in the set. This can be used to pick a peer
// for block requests, preserving the peers that have the highest coverage scores
// for column requests.
func (m *ColumnScarcityRanking) ForBlocks(busy map[peer.ID]bool) (peer.ID, error) {
	for i := len(m.covScoreRank) - 1; i >= 0; i-- {
		p := m.covScoreRank[i]
		if !busy[p.peerID] {
			return p.peerID, nil
		}
	}
	return "", errors.New("no peers available")
}

// peerColumnCoverage represents a peer's custody of columns and their coverage score.
type peerColumnCoverage struct {
	peerID    peer.ID
	nodeID    enode.ID
	custodied []uint64
	cov       float64
}

func (p *peerColumnCoverage) covered(needed peerdas.ColumnIndices) []uint64 {
	covered := make([]uint64, 0, len(p.custodied))
	for col, want := range needed {
		if want && p.custodied[col] == 1 {
			covered = append(covered, col)
		}
	}
	return covered
}

func (p *peerColumnCoverage) score(scarcity []float64) float64 {
	if p.cov == 0 {
		p.cov = coverageScore(p.custodied, scarcity)
	}
	return p.cov
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

type colFreq struct {
	col        uint64
	custodians []*peerColumnCoverage
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

func columnScarcity(cf []colFreq) []float64 {
	ra := make([]float64, len(cf))
	for _, f := range cf {
		ra[f.col] = f.rarity()
	}
	return ra
}
