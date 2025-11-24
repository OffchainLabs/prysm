package sync

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

var (
	// ErrNoPeersCoverNeeded is returned when no peers are able to cover the needed columns.
	ErrNoPeersCoverNeeded = errors.New("no peers able to cover needed columns")
	// ErrNoPeersAvailable is returned when no peers are available for block requests.
	ErrNoPeersAvailable = errors.New("no peers available")
)

// DASPeerCache caches information about a set of peers DAS peering decisions.
type DASPeerCache struct {
	p2pSvc p2p.P2P
	peers  map[peer.ID]*dasPeer
}

// dasPeer represents a peer's custody of columns and their coverage score.
type dasPeer struct {
	pid          peer.ID
	enid         enode.ID
	custodied    peerdas.ColumnIndices
	lastAssigned time.Time
}

// dasPeerScore is used to build a slice of peer+score pairs for ranking purproses.
type dasPeerScore struct {
	peer  *dasPeer
	score float64
}

// PeerPicker is a structure that maps out the intersection of peer custody and column indices
// to weight each peer based on the scarcity of the columns they custody. This allows us to prioritize
// requests for more scarce columns to peers that custody them, so that we don't waste our bandwidth allocation
// making requests for more common columns from peers that can provide the more scarce columns.
type PeerPicker struct {
	scores      []*dasPeerScore // scores is a set of generic scores, based on the full custody group set
	ranker      *rarityRanker
	custodians  map[uint64][]*dasPeer
	toCustody   peerdas.ColumnIndices // full set of columns this node will try to custody
	reqInterval time.Duration
}

// NewDASPeerCache initializes a DASPeerCache. This type is not currently thread safe.
func NewDASPeerCache(p2pSvc p2p.P2P) *DASPeerCache {
	return &DASPeerCache{
		peers:  make(map[peer.ID]*dasPeer),
		p2pSvc: p2pSvc,
	}
}

// NewColumnScarcityRanking computes the ColumnScarcityRanking based on the current view of columns custodied
// by the given set of peers. New PeerPickers should be created somewhat frequently, as the status of peers can
// change, including the set of columns each peer custodies.
// reqInterval sets the frequency that a peer can be picked in terms of time. A peer can be picked once per reqInterval,
// eg a value of time.Second would allow 1 request per second to the peer, or a value of 500 * time.Millisecond would allow
// 2 req/sec.
func (c *DASPeerCache) NewPicker(pids []peer.ID, toCustody peerdas.ColumnIndices, reqInterval time.Duration) (*PeerPicker, error) {
	// For each of the given peers, refresh the cache's view of their currently custodied columns.
	// Also populate 'custodians', which stores the set of peers that custody each column index.
	custodians := make(map[uint64][]*dasPeer, len(toCustody))
	scores := make([]*dasPeerScore, 0, len(pids))
	for _, pid := range pids {
		peer, err := c.refresh(pid, toCustody)
		if err != nil {
			log.WithField("peerID", pid).WithError(err).Debug("Failed to convert peer ID to node ID.")
			continue
		}
		for col := range peer.custodied {
			if toCustody.Has(col) {
				custodians[col] = append(custodians[col], peer)
			}
		}
		// set score to math.MaxFloat64 so we can tell that it hasn't been initialized
		scores = append(scores, &dasPeerScore{peer: peer, score: math.MaxFloat64})
	}

	return &PeerPicker{
		toCustody:   toCustody,
		ranker:      newRarityRanker(toCustody, custodians),
		custodians:  custodians,
		scores:      scores,
		reqInterval: reqInterval,
	}, nil
}

// refresh supports NewPicker in getting the latest dasPeer view for the given peer.ID. It caches the result
// of the enode.ID computation but refreshes the custody group count each time it is called, leveraging the
// cache behind peerdas.Info.
func (c *DASPeerCache) refresh(pid peer.ID, toCustody peerdas.ColumnIndices) (*dasPeer, error) {
	// Computing the enode.ID seems to involve multiple parseing and validation steps followed by a
	// hash computation, so it seems worth trying to cache the result.
	p, ok := c.peers[pid]
	if !ok {
		nodeID, err := p2p.ConvertPeerIDToNodeID(pid)
		if err != nil {
			// If we can't convert the peer ID to a node ID, remove peer from the cache.
			delete(c.peers, pid)
			return nil, errors.Wrap(err, "ConvertPeerIDToNodeID")
		}
		p = &dasPeer{enid: nodeID, pid: pid}
	}
	if len(toCustody) > 0 {
		dasInfo, _, err := peerdas.Info(p.enid, c.p2pSvc.CustodyGroupCountFromPeer(pid))
		if err != nil {
			// If we can't get the peerDAS info, remove peer from the cache.
			delete(c.peers, pid)
			return nil, errors.Wrapf(err, "CustodyGroupCountFromPeer, peerID=%s, nodeID=%s", pid, p.enid)
		}
		p.custodied = peerdas.NewColumnIndicesFromMap(dasInfo.CustodyColumns)
	} else {
		p.custodied = peerdas.NewColumnIndices()
	}
	c.peers[pid] = p
	return p, nil
}

// ForColumns returns the best peer to request columns from, based on the scarcity of the columns needed.
func (m *PeerPicker) ForColumns(needed peerdas.ColumnIndices, busy map[peer.ID]bool) (peer.ID, []uint64, error) {
	// - find the custodied column with the lowest frequency
	// - collect all the peers that have custody of that column
	// - score the peers by the rarity of the needed columns they offer
	var best *dasPeer
	bestScore, bestCoverage := 0.0, []uint64{}
	for _, col := range m.ranker.ascendingRarity(needed) {
		for _, p := range m.custodians[col] {
			// enforce a minimum interval between requests to the same peer
			if p.lastAssigned.Add(m.reqInterval).After(time.Now()) {
				continue
			}
			if busy[p.pid] {
				continue
			}
			covered := p.custodied.Intersection(needed)
			if len(covered) == 0 {
				continue
			}
			// update best if any of the following:
			// - current score better than previous best
			// - scores are tied, and current coverage is better than best
			// - scores are tied, coverage equal, pick the least-recently used peer
			score := m.ranker.score(covered)
			if score < bestScore {
				continue
			}
			if score == bestScore && best != nil {
				if len(covered) < len(bestCoverage) {
					continue
				}
				if len(covered) == len(bestCoverage) && best.lastAssigned.Before(p.lastAssigned) {
					continue
				}
			}
			best, bestScore, bestCoverage = p, score, covered.ToSlice()
		}
		if best != nil {
			best.lastAssigned = time.Now()
			slices.Sort(bestCoverage)
			return best.pid, bestCoverage, nil
		}
	}

	return "", nil, ErrNoPeersCoverNeeded
}

// ForBlocks returns the lowest scoring peer in the set. This can be used to pick a peer
// for block requests, preserving the peers that have the highest coverage scores
// for column requests.
func (m *PeerPicker) ForBlocks(busy map[peer.ID]bool) (peer.ID, error) {
	slices.SortFunc(m.scores, func(a, b *dasPeerScore) int {
		// MaxFloat64 is used as a sentinel value for an uninitialized score;
		// check and set scores while sorting for uber-lazy initialization.
		if a.score == math.MaxFloat64 {
			a.score = m.ranker.score(a.peer.custodied.Intersection(m.toCustody))
		}
		if b.score == math.MaxFloat64 {
			b.score = m.ranker.score(b.peer.custodied.Intersection(m.toCustody))
		}
		return cmp.Compare(a.score, b.score)
	})
	for _, ds := range m.scores {
		if !busy[ds.peer.pid] {
			return ds.peer.pid, nil
		}
	}
	return "", ErrNoPeersAvailable
}

// rarityRanker is initialized with the set of columns this node needs to custody, and the set of
// all peer custody columns. With that information it is able to compute a numeric representation of
// column rarity, and use that number to give each peer a score that represents how fungible their
// bandwidth likely is relative to other peers given a more specific set of needed columns.
type rarityRanker struct {
	// rarity maps column indices to their rarity scores.
	// The rarity score is defined as the inverse of the number of custodians: 1/custodians
	// So the rarity of the columns a peer custodies can be simply added together for a score
	// representing how unique their custody groups are; rarer columns contribute larger values to scores.
	rarity map[uint64]float64
	asc    []uint64 // columns indices ordered by ascending rarity
}

// newRarityRanker precomputes data used for scoring and ranking. It should be reinitialized every time
// we refresh the set of peers or the view of the peers column custody.
func newRarityRanker(toCustody peerdas.ColumnIndices, custodians map[uint64][]*dasPeer) *rarityRanker {
	rarity := make(map[uint64]float64, len(toCustody))
	asc := make([]uint64, 0, len(toCustody))
	for col := range toCustody.ToMap() {
		rarity[col] = 1 / max(1, float64(len(custodians[col])))
		asc = append(asc, col)
	}
	slices.SortFunc(asc, func(a, b uint64) int {
		return cmp.Compare(rarity[a], rarity[b])
	})
	return &rarityRanker{rarity: rarity, asc: asc}
}

// rank returns the requested columns sorted by ascending rarity.
func (rr *rarityRanker) ascendingRarity(cols peerdas.ColumnIndices) []uint64 {
	ranked := make([]uint64, 0, len(cols))
	for _, col := range rr.asc {
		if cols.Has(col) {
			ranked = append(ranked, col)
		}
	}
	return ranked
}

// score gives a score representing the sum of the rarity scores of the given columns. It can be used to
// score peers based on the set intersection of their custodied indices and the indices we need to request.
func (rr *rarityRanker) score(coverage peerdas.ColumnIndices) float64 {
	score := 0.0
	for col := range coverage.ToMap() {
		score += rr.rarity[col]
	}
	return score
}
