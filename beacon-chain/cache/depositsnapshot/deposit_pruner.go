package depositsnapshot

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// PruneProofs removes proofs from all deposits whose index is equal or less than untilDepositIndex.
func (c *Cache) PruneProofs(ctx context.Context, untilDepositIndex int64) error {
	_, span := trace.StartSpan(ctx, "Cache.PruneProofs")
	defer span.End()
	c.depositsLock.Lock()
	defer c.depositsLock.Unlock()

	if untilDepositIndex >= int64(len(c.deposits)) {
		untilDepositIndex = int64(len(c.deposits) - 1)
	}

	for i := untilDepositIndex; i >= 0; i-- {
		// Finding a nil proof means that all proofs up to this deposit have been already pruned.
		if c.deposits[i].Deposit.Proof == nil {
			break
		}
		c.deposits[i].Deposit.Proof = nil
	}

	return nil
}

// PruneAllProofs removes proofs from all deposits.
// As EIP-6110 applies and the legacy deposit mechanism is deprecated,
// proofs in deposit snapshot are no longer needed.
// See: https://eips.ethereum.org/EIPS/eip-6110#eth1data-poll-deprecation
func (c *Cache) PruneAllProofs(ctx context.Context) {
	_, span := trace.StartSpan(ctx, "Cache.PruneAllProofs")
	defer span.End()

	c.depositsLock.Lock()
	defer c.depositsLock.Unlock()

	for i := len(c.deposits) - 1; i >= 0; i-- {
		if c.deposits[i].Deposit.Proof == nil {
			break
		}
		c.deposits[i].Deposit.Proof = nil
	}
}

// PrunePendingDeposits removes any deposit which is older than the given deposit merkle tree index.
func (c *Cache) PrunePendingDeposits(ctx context.Context, merkleTreeIndex int64) {
	_, span := trace.StartSpan(ctx, "Cache.PrunePendingDeposits")
	defer span.End()

	if merkleTreeIndex == 0 {
		log.Debug("Ignoring 0 deposit removal")
		return
	}

	c.depositsLock.Lock()
	defer c.depositsLock.Unlock()

	cleanDeposits := make([]*ethpb.DepositContainer, 0, len(c.pendingDeposits))
	for _, dp := range c.pendingDeposits {
		if dp.Index >= merkleTreeIndex {
			cleanDeposits = append(cleanDeposits, dp)
		}
	}

	c.pendingDeposits = cleanDeposits
	pendingDepositsCount.Set(float64(len(c.pendingDeposits)))
}

// PruneAllPendingDeposits removes all pending deposits from the cache.
// As EIP-6110 applies and the legacy deposit mechanism is deprecated,
// pending deposits in deposit snapshot are no longer needed.
// See: https://eips.ethereum.org/EIPS/eip-6110#eth1data-poll-deprecation
func (c *Cache) PruneAllPendingDeposits(ctx context.Context) {
	_, span := trace.StartSpan(ctx, "Cache.PruneAllPendingDeposits")
	defer span.End()

	c.depositsLock.Lock()
	defer c.depositsLock.Unlock()

	c.pendingDeposits = make([]*ethpb.DepositContainer, 0)
	pendingDepositsCount.Set(float64(0))
}
