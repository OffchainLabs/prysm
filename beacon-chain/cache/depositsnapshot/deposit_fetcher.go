package depositsnapshot

import (
	"context"
	"math/big"
	"sort"
	"sync"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"github.com/wealdtech/go-bytesutil"
)

var (
	pendingDepositsCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacondb_pending_deposits_eip4881",
		Help: "The number of pending deposits in memory",
	})
)

// Cache stores all in-memory deposit objects. This
// stores all the deposit related data that is required by the beacon-node.
type Cache struct {
	pendingDeposits   []*ethpb.DepositContainer
	deposits          []*ethpb.DepositContainer
	finalizedDeposits finalizedDepositsContainer
	depositsByKey     map[[fieldparams.BLSPubkeyLength]byte][]*ethpb.DepositContainer
	depositsLock      sync.RWMutex
}

// finalizedDepositsContainer stores the trie of deposits that have been included
// in the beacon state up to the latest finalized checkpoint.
type finalizedDepositsContainer struct {
	depositTree     *DepositTree
	merkleTrieIndex int64
}

// New instantiates a new deposit cache
func New() (*Cache, error) {
	finalizedDepositsTrie := NewDepositTree()

	// finalizedDeposits.merkleTrieIndex is initialized to -1 because it represents the index of the last trie item.
	// Inserting the first item into the trie will set the value of the index to 0.
	return &Cache{
		pendingDeposits:   []*ethpb.DepositContainer{},
		deposits:          []*ethpb.DepositContainer{},
		depositsByKey:     map[[fieldparams.BLSPubkeyLength]byte][]*ethpb.DepositContainer{},
		finalizedDeposits: toFinalizedDepositsContainer(finalizedDepositsTrie, -1),
	}, nil
}

// AllDeposits returns a list of historical deposits until the given block number
// (inclusive). If no block is specified then this method returns all historical deposits.
func (c *Cache) AllDeposits(ctx context.Context, untilBlk *big.Int) []*ethpb.Deposit {
	c.depositsLock.RLock()
	defer c.depositsLock.RUnlock()

	return c.allDeposits(untilBlk)
}

func (c *Cache) allDeposits(untilBlk *big.Int) []*ethpb.Deposit {
	var deposits []*ethpb.Deposit
	for _, ctnr := range c.deposits {
		cBlk := big.NewInt(0).SetUint64(ctnr.Eth1BlockHeight)
		if untilBlk == nil || untilBlk.Cmp(cBlk) >= 0 {
			deposits = append(deposits, ctnr.Deposit)
		}
	}
	return deposits
}

// AllDepositContainers returns all historical deposit containers.
func (c *Cache) AllDepositContainers(ctx context.Context) []*ethpb.DepositContainer {
	_, span := trace.StartSpan(ctx, "Cache.AllDepositContainers")
	defer span.End()
	c.depositsLock.RLock()
	defer c.depositsLock.RUnlock()

	// Make a shallow copy of the deposits and return that. This way, the
	// caller can safely iterate over the returned list of deposits without
	// the possibility of new deposits showing up. If we were to return the
	// list without a copy, when a new deposit is added to the cache, it
	// would also be present in the returned value. This could result in a
	// race condition if the list is being iterated over.
	//
	// It's not necessary to make a deep copy of this list because the
	// deposits in the cache should never be modified. It is still possible
	// for the caller to modify one of the underlying deposits and modify
	// the cache, but that's not a race condition. Also, a deep copy would
	// take too long and use too much memory.
	deposits := make([]*ethpb.DepositContainer, len(c.deposits))
	copy(deposits, c.deposits)
	return deposits
}

// DepositByPubkey looks through historical deposits and finds one which contains
// a certain public key within its deposit data.
func (c *Cache) DepositByPubkey(ctx context.Context, pubKey []byte) (*ethpb.Deposit, *big.Int) {
	_, span := trace.StartSpan(ctx, "Cache.DepositByPubkey")
	defer span.End()
	c.depositsLock.RLock()
	defer c.depositsLock.RUnlock()

	var deposit *ethpb.Deposit
	var blockNum *big.Int
	deps, ok := c.depositsByKey[bytesutil.ToBytes48(pubKey)]
	if !ok || len(deps) == 0 {
		return deposit, blockNum
	}
	// We always return the first deposit if a particular
	// validator key has multiple deposits assigned to
	// it.
	deposit = deps[0].Deposit
	blockNum = big.NewInt(int64(deps[0].Eth1BlockHeight))
	return deposit, blockNum
}

// DepositsNumberAndRootAtHeight returns number of deposits made up to blockheight and the
// root that corresponds to the latest deposit at that blockheight.
func (c *Cache) DepositsNumberAndRootAtHeight(ctx context.Context, blockHeight *big.Int) (uint64, [32]byte) {
	_, span := trace.StartSpan(ctx, "Cache.DepositsNumberAndRootAtHeight")
	defer span.End()
	c.depositsLock.RLock()
	defer c.depositsLock.RUnlock()
	heightIdx := sort.Search(len(c.deposits), func(i int) bool {
		dBlkHeight := big.NewInt(0).SetUint64(c.deposits[i].Eth1BlockHeight)
		return dBlkHeight.Cmp(blockHeight) > 0
	})
	// send the deposit root of the empty trie, if eth1follow distance is greater than the time of the earliest
	// deposit.
	if heightIdx == 0 {
		return 0, [32]byte{}
	}
	return uint64(heightIdx), bytesutil.ToBytes32(c.deposits[heightIdx-1].DepositRoot)
}

// FinalizedDeposits returns the finalized deposits trie.
func (c *Cache) FinalizedDeposits(ctx context.Context) (cache.FinalizedDeposits, error) {
	_, span := trace.StartSpan(ctx, "Cache.FinalizedDeposits")
	defer span.End()
	c.depositsLock.RLock()
	defer c.depositsLock.RUnlock()

	tree, err := c.finalizedDeposits.depositTree.Copy()
	if err != nil {
		return nil, err
	}
	return &finalizedDepositsContainer{
		depositTree:     tree,
		merkleTrieIndex: c.finalizedDeposits.merkleTrieIndex,
	}, nil
}

// NonFinalizedDeposits returns the list of non-finalized deposits until the given block number (inclusive).
// If no block is specified then this method returns all non-finalized deposits.
func (c *Cache) NonFinalizedDeposits(ctx context.Context, lastFinalizedIndex int64, untilBlk *big.Int) []*ethpb.Deposit {
	_, span := trace.StartSpan(ctx, "Cache.NonFinalizedDeposits")
	defer span.End()
	c.depositsLock.RLock()
	defer c.depositsLock.RUnlock()

	if c.finalizedDeposits.depositTree == nil {
		return c.allDeposits(untilBlk)
	}

	var deposits []*ethpb.Deposit
	for _, d := range c.deposits {
		if (d.Index > lastFinalizedIndex) && (untilBlk == nil || untilBlk.Uint64() >= d.Eth1BlockHeight) {
			deposits = append(deposits, d.Deposit)
		}
	}

	return deposits
}

// InsertPendingDeposit into the database. If deposit or block number are nil
// then this method does nothing.
func (c *Cache) InsertPendingDeposit(ctx context.Context, d *ethpb.Deposit, blockNum uint64, index int64, depositRoot [32]byte) {
	_, span := trace.StartSpan(ctx, "Cache.InsertPendingDeposit")
	defer span.End()
	if d == nil {
		log.WithFields(logrus.Fields{
			"block":   blockNum,
			"deposit": d,
		}).Debug("Ignoring nil deposit insertion")
		return
	}
	c.depositsLock.Lock()
	defer c.depositsLock.Unlock()
	c.pendingDeposits = append(c.pendingDeposits,
		&ethpb.DepositContainer{Deposit: d, Eth1BlockHeight: blockNum, Index: index, DepositRoot: depositRoot[:]})
	pendingDepositsCount.Set(float64(len(c.pendingDeposits)))
	span.SetAttributes(trace.Int64Attribute("count", int64(len(c.pendingDeposits))))
}

// Deposits returns the cached internal deposit tree.
func (fd *finalizedDepositsContainer) Deposits() cache.MerkleTree {
	return fd.depositTree
}

// MerkleTrieIndex represents the last finalized index in
// the finalized deposit container.
func (fd *finalizedDepositsContainer) MerkleTrieIndex() int64 {
	return fd.merkleTrieIndex
}

func toFinalizedDepositsContainer(deposits *DepositTree, index int64) finalizedDepositsContainer {
	return finalizedDepositsContainer{
		depositTree:     deposits,
		merkleTrieIndex: index,
	}
}

// PendingDepositsFetcher specifically outlines a struct that can retrieve deposits
// which have not yet been included in the chain.
type PendingDepositsFetcher interface {
	PendingContainers(ctx context.Context, untilBlk *big.Int) []*ethpb.DepositContainer
}

// PendingDeposits returns a list of deposits until the given block number
// (inclusive). If no block is specified then this method returns all pending
// deposits.
func (c *Cache) PendingDeposits(ctx context.Context, untilBlk *big.Int) []*ethpb.Deposit {
	ctx, span := trace.StartSpan(ctx, "Cache.PendingDeposits")
	defer span.End()

	depositCntrs := c.PendingContainers(ctx, untilBlk)

	deposits := make([]*ethpb.Deposit, 0, len(depositCntrs))
	for _, dep := range depositCntrs {
		deposits = append(deposits, dep.Deposit)
	}

	return deposits
}

// PendingContainers returns a list of deposit containers until the given block number
// (inclusive).
func (c *Cache) PendingContainers(ctx context.Context, untilBlk *big.Int) []*ethpb.DepositContainer {
	_, span := trace.StartSpan(ctx, "Cache.PendingContainers")
	defer span.End()
	c.depositsLock.RLock()
	defer c.depositsLock.RUnlock()

	depositCntrs := make([]*ethpb.DepositContainer, 0, len(c.pendingDeposits))
	for _, ctnr := range c.pendingDeposits {
		if untilBlk == nil || untilBlk.Uint64() >= ctnr.Eth1BlockHeight {
			depositCntrs = append(depositCntrs, ctnr)
		}
	}
	// Sort the deposits by Merkle index.
	sort.SliceStable(depositCntrs, func(i, j int) bool {
		return depositCntrs[i].Index < depositCntrs[j].Index
	})

	span.SetAttributes(trace.Int64Attribute("count", int64(len(depositCntrs))))

	return depositCntrs
}
