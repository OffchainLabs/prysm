package backfill

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

var errMaxBatches = errors.New("backfill batch requested in excess of max outstanding batches")
var errEndSequence = errors.New("sequence has terminated, no more backfill batches will be produced")
var errCannotDecreaseMinimum = errors.New("the minimum backfill slot can only be increased, not decreased")

type batchSequencer struct {
	batcher      batcher
	seq          []batch
	currentNeeds func() das.CurrentNeeds
}

// sequence() is meant as a verb "arrange in a particular order".
// sequence determines the next set of batches that should be worked on based on the state of the batches
// in its internal view. sequence relies on update() for updates to its view of the
// batches it has previously sequenced.
func (c *batchSequencer) sequence() ([]batch, error) {
	needs := c.currentNeeds()
	s := make([]batch, 0)
	// batch start slots are in descending order, c.seq[n].begin == c.seq[n+1].end
	for i := range c.seq {
		if c.seq[i].state == batchNil {
			// batchNil is the zero value of the batch type.
			// This case means that we are initializing a batch that was created by the
			// initial allocation of the seq slice, so batcher need to compute its bounds.
			if i == 0 {
				// The first item in the list is a special case, subsequent items are initialized
				// relative to the preceding batches.
				c.seq[i] = c.batcher.before(c.batcher.max)
			} else {
				c.seq[i] = c.batcher.beforeBatch(c.seq[i-1])
			}
		}
		if c.seq[i].state == batchInit || c.seq[i].state == batchErrRetryable {
			// This means the batch has fallen outside the retention window so we no longer need to sync it.
			// Since we always create batches from high to low, we can assume we've already created the
			// descendent batches from the batch we're dropping, so there won't be another batch depending on
			// this one - we can stop adding batches and mark put this one in the batchEndSequence state.
			// When all batches are in batchEndSequence, worker pool spins down and marks backfill complete.
			if c.seq[i].expired(needs) {
				c.seq[i] = c.seq[i].withState(batchEndSequence)
			} else {
				c.seq[i] = c.seq[i].withState(batchSequenced)
				s = append(s, c.seq[i])
				continue
			}
		}
		if c.seq[i].state == batchEndSequence && len(s) == 0 {
			s = append(s, c.seq[i])
			continue
		}
	}
	if len(s) == 0 {
		return nil, errMaxBatches
	}

	return s, nil
}

// update serves 2 roles.
//   - updating batchSequencer's copy of the given batch.
//   - removing batches that are completely imported from the sequence,
//     so that they are not returned the next time import() is called, and populating
//     seq with new batches that are ready to be worked on.
func (c *batchSequencer) update(b batch) {
	done := 0
	needs := c.currentNeeds()
	for i := 0; i < len(c.seq); i++ {
		if b.replaces(c.seq[i]) {
			c.seq[i] = b
		}
		// Assumes invariant that batches complete and update is called in order.
		// This should be true because the code using the sequencer doesn't know the expected parent
		// for a batch until it imports the previous batch.
		if c.seq[i].state == batchImportComplete {
			done += 1
			continue
		}

		if c.seq[i].expired(needs) {
			c.seq[i] = c.seq[i].withState(batchEndSequence)
			done += 1
			continue
		}
		// Move the unfinished batches to overwrite the finished ones.
		// eg consider [a,b,c,d,e] where a,b are done
		// when i==2, done==2 (since done was incremented for a and b)
		// so we want to copy c to a, then on i=3, d to b, then on i=4 e to c.
		c.seq[i-done] = c.seq[i]
	}
	if done == len(c.seq) {
		c.seq[0] = c.batcher.beforeBatch(c.seq[0])
		return
	}

	// Overwrite the moved batches with the next ones in the sequence.
	// Continuing the example in the comment above, len(c.seq)==5, done=2, so i=3.
	// We want to replace index 3 with the batch that should be processed after index 2,
	// which was previously the earliest known batch, and index 4 with the batch that should
	// be processed after index 3, the new earliest batch.
	for i := len(c.seq) - done; i < len(c.seq); i++ {
		c.seq[i] = c.batcher.beforeBatch(c.seq[i-1])
	}
}

// importable returns all batches that are ready to be imported. This means they satisfy 2 conditions:
//   - They are in state batchImportable, which means their data has been downloaded and proposer signatures have been verified.
//   - There are no batches that are not in state batchImportable between them and the start of the slice. This ensures that they
//     can be connected to the canonical chain, either because the root of the last block in the batch matches the parent_root of
//     the oldest block in the canonical chain, or because the root of the last block in the batch matches the parent_root of the
//     new block preceding them in the slice (which must connect to the batch before it, or to the canonical chain if it is first).
func (c *batchSequencer) importable() []batch {
	imp := make([]batch, 0)
	for i := range c.seq {
		if c.seq[i].state == batchImportable {
			imp = append(imp, c.seq[i])
			continue
		}
		// as soon as we hit a batch with a different state, we return everything leading to it.
		// If the first element isn't importable, we'll return an empty slice.
		break
	}
	return imp
}

// countWithState provides a view into how many batches are in a particular state
// to be used for logging or metrics purposes.
func (c *batchSequencer) countWithState(s batchState) int {
	n := 0
	for i := 0; i < len(c.seq); i++ {
		if c.seq[i].state == s {
			n += 1
		}
	}
	return n
}

// numTodo computes the number of remaining batches for metrics and logging purposes.
func (c *batchSequencer) numTodo() int {
	if len(c.seq) == 0 {
		return 0
	}
	lowest := c.seq[len(c.seq)-1]
	todo := 0
	if lowest.state != batchEndSequence {
		todo = c.batcher.remaining(lowest.begin)
	}
	for _, b := range c.seq {
		switch b.state {
		case batchEndSequence, batchImportComplete, batchNil:
			continue
		default:
			todo += 1
		}
	}
	return todo
}

func newBatchSequencer(seqLen int, max, size primitives.Slot, needsCb func() das.CurrentNeeds) *batchSequencer {
	b := batcher{currentNeeds: needsCb, max: max, size: size}
	seq := make([]batch, seqLen)
	return &batchSequencer{batcher: b, seq: seq, currentNeeds: needsCb}
}

type batcher struct {
	currentNeeds func() das.CurrentNeeds
	max          primitives.Slot
	size         primitives.Slot
}

func (r batcher) remaining(upTo primitives.Slot) int {
	needs := r.currentNeeds()
	if !needs.Block.At(upTo) {
		return 0
	}
	delta := upTo - needs.Block.Begin
	if delta%r.size != 0 {
		return int(delta/r.size) + 1
	}
	return int(delta / r.size)
}

func (r batcher) beforeBatch(upTo batch) batch {
	return r.before(upTo.begin)
}

func (r batcher) before(upTo primitives.Slot) batch {
	// upTo is an exclusive upper bound. If we do not need the block at the upTo slot,
	// we don't have anything left to sync, signaling the end of the backfill process.
	needs := r.currentNeeds()
	// The upper bound is exclusive, so we shouldn't return in this case where the previous
	// batch beginning sits at the exact slot of the start of the retention window. In that case
	// we've actually hit the end of the sync sequence.
	if !needs.Block.At(upTo) || needs.Block.Begin == upTo {
		return batch{begin: upTo, end: upTo, state: batchEndSequence}
	}

	begin := needs.Block.Begin
	if upTo > r.size+needs.Block.Begin {
		begin = upTo - r.size
	}

	// batch.end is exclusive, .begin is inclusive, so the prev.end = next.begin
	return batch{begin: begin, end: upTo, state: batchInit}
}
