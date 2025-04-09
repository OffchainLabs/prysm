package depositsnapshot

import (
	"context"
	"encoding/hex"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/sirupsen/logrus"
)

var (
	historicalDepositsCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacondb_all_deposits_eip4881",
		Help: "The number of total deposits in memory",
	})
	log = logrus.WithField("prefix", "cache")
)

// InsertDeposit into the database. If deposit or block number are nil
// then this method does nothing.
func (c *Cache) InsertDeposit(ctx context.Context, d *ethpb.Deposit, blockNum uint64, index int64, depositRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "Cache.InsertDeposit")
	defer span.End()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if d == nil {
		log.WithFields(logrus.Fields{
			"block":       blockNum,
			"deposit":     d,
			"index":       index,
			"depositRoot": hex.EncodeToString(depositRoot[:]),
		}).Warn("Ignoring nil deposit insertion")
		return errors.New("nil deposit inserted into the cache")
	}
	c.depositsLock.Lock()
	defer c.depositsLock.Unlock()

	if int(index) != len(c.deposits) {
		return errors.Errorf("wanted deposit with index %d to be inserted but received %d", len(c.deposits), index)
	}
	// Keep the slice sorted on insertion in order to avoid costly sorting on retrieval.
	heightIdx := sort.Search(len(c.deposits), func(i int) bool { return c.deposits[i].Index >= index })
	depCtr := &ethpb.DepositContainer{Deposit: d, Eth1BlockHeight: blockNum, DepositRoot: depositRoot[:], Index: index}
	newDeposits := append(
		[]*ethpb.DepositContainer{depCtr},
		c.deposits[heightIdx:]...)
	c.deposits = append(c.deposits[:heightIdx], newDeposits...)
	// Append the deposit to our map, in the event no deposits
	// exist for the pubkey , it is simply added to the map.
	pubkey := bytesutil.ToBytes48(d.Data.PublicKey)
	c.depositsByKey[pubkey] = append(c.depositsByKey[pubkey], depCtr)
	historicalDepositsCount.Inc()
	return nil
}

// InsertDepositContainers inserts a set of deposit containers into our deposit cache.
func (c *Cache) InsertDepositContainers(ctx context.Context, ctrs []*ethpb.DepositContainer) {
	_, span := trace.StartSpan(ctx, "Cache.InsertDepositContainers")
	defer span.End()
	c.depositsLock.Lock()
	defer c.depositsLock.Unlock()

	// Initialize slice if nil object provided.
	if ctrs == nil {
		ctrs = make([]*ethpb.DepositContainer, 0)
	}
	sort.SliceStable(ctrs, func(i int, j int) bool { return ctrs[i].Index < ctrs[j].Index })
	c.deposits = ctrs
	for _, ctr := range ctrs {
		// Use a new value, as the reference
		// changes in the next iteration.
		newPtr := ctr
		pKey := bytesutil.ToBytes48(newPtr.Deposit.Data.PublicKey)
		c.depositsByKey[pKey] = append(c.depositsByKey[pKey], newPtr)
	}
	historicalDepositsCount.Add(float64(len(ctrs)))
}

// InsertFinalizedDeposits inserts deposits up to eth1DepositIndex (inclusive) into the finalized deposits cache.
func (c *Cache) InsertFinalizedDeposits(ctx context.Context, eth1DepositIndex int64,
	executionHash common.Hash, executionNumber uint64) error {
	ctx, span := trace.StartSpan(ctx, "Cache.InsertFinalizedDeposits")
	defer span.End()
	c.depositsLock.Lock()
	defer c.depositsLock.Unlock()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	depositTrie := c.finalizedDeposits.depositTree
	insertIndex := int(c.finalizedDeposits.MerkleTrieIndex() + 1)

	// Don't insert into finalized trie if there is no deposit to
	// insert.
	if len(c.deposits) == 0 {
		return nil
	}
	// In the event we have less deposits than we need to
	// finalize we finalize till the index on which we do have it.
	if len(c.deposits) <= int(eth1DepositIndex) {
		eth1DepositIndex = int64(len(c.deposits)) - 1
	}
	// If we finalize to some lower deposit index, we
	// ignore it.
	if int(eth1DepositIndex) < insertIndex {
		return nil
	}
	currIdx := int64(depositTrie.depositCount) - 1

	// Insert deposits into deposit trie.
	for _, ctr := range c.deposits {
		if ctr.Index > currIdx && ctr.Index <= eth1DepositIndex {
			rt, err := ctr.Deposit.Data.HashTreeRoot()
			if err != nil {
				return err
			}
			if err := depositTrie.Insert(rt[:], int(ctr.Index)); err != nil {
				return err
			}
		}
	}

	if err := depositTrie.Finalize(eth1DepositIndex, executionHash, executionNumber); err != nil {
		return err
	}

	c.finalizedDeposits = finalizedDepositsContainer{
		depositTree:     depositTrie,
		merkleTrieIndex: eth1DepositIndex,
	}
	return nil
}
