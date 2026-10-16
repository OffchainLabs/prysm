package decoupled

import (
	"bytes"
	"errors"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

var ErrUnknownParent = errors.New("block parent is not in the store")

// The fields InsertNode reads until the Decoupled state and block getters exist. The store keeps the vote slices, the caller must not reuse them.
type Block struct {
	Root         [32]byte
	Parent       [32]byte
	Slot         primitives.Slot
	BlockVotes   []Vote
	SupportVotes []Vote
	FG           FGSummary
}

// Spec: fg_on_block then ac_on_block. P2P and the state transition already validated the block, only the parent must be a node. Due ticks run first so they see the tree as it was at their instant.
func (st *Store) InsertNode(b Block, now time.Time) error {
	st.OnTick(now)
	if st.nodes[b.Root] != nil {
		return nil
	}
	parent := st.nodes[b.Parent]
	if parent == nil {
		return ErrUnknownParent
	}
	n := st.addChild(b.Root, parent, b.Slot)
	n.processed = now
	n.BlockVotes = b.BlockVotes
	n.SupportVotes = b.SupportVotes
	n.FG = b.FG
	st.updateFinality(n)
	for _, v := range b.BlockVotes {
		st.addVote(v, now)
	}
	return nil
}

// Spec: the (h_j, J.root) lexicographic order, greatest root wins a tie.
func (p Pair) less(q Pair) bool {
	if p.Height != q.Height {
		return p.Height < q.Height
	}
	return bytes.Compare(p.Root[:], q.Root[:]) < 0
}

// Spec: update_finality. Three statements in order, each reading the store the previous one wrote.
func (st *Store) updateFinality(n *Node) {
	st.hMax = max(st.hMax, n.FG.Target.Height)
	j := st.nodes[n.FG.Justified.Root]
	current := Pair{Height: st.hJ, Root: st.justified.Root}
	if j != nil && current.less(n.FG.Justified) {
		st.justified, st.hJ = j, n.FG.Justified.Height
	}
	f := st.nodes[n.FG.Finalized.Root]
	if f != nil && f != st.finalized && isAncestor(f, st.justified) && st.hasViableDescendant(f) {
		st.finalized, st.hF = f, n.FG.Finalized.Height
		st.prune()
	}
}

func (st *Store) Finalized() *Node                   { return st.finalized }
func (st *Store) Justified() *Node                   { return st.justified }
func (st *Store) JustifiedHeight() primitives.Height { return st.hJ }
func (st *Store) FinalizedHeight() primitives.Height { return st.hF }
func (st *Store) MaxHeight() primitives.Height       { return st.hMax }
func (st *Store) CommitteeSize() uint64              { return st.cfg.CommitteeSize }
func (st *Store) Joined(slot primitives.Slot) bool   { return st.joined(slot) }

// Distinct validators whose vote for the slot the pool holds.
func (st *Store) Voters(slot primitives.Slot) int {
	if b := st.pool[slot]; b != nil {
		return len(b.votesBy)
	}
	return 0
}

func (st *Store) Locks() []Lock {
	out := make([]Lock, 0, len(st.locks))
	for _, l := range st.locks {
		out = append(out, Lock{Root: l.block.Root, Slot: l.block.Slot, Deadline: l.deadline})
	}
	return out
}
