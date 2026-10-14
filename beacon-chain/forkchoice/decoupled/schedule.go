package decoupled

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

func (st *Store) slotStart(s primitives.Slot) time.Time {
	return st.genesis.Add(4 * st.cfg.Delta * time.Duration(s))
}

func (st *Store) slotAt(now time.Time) primitives.Slot {
	if now.Before(st.genesis) {
		return 0
	}
	return primitives.Slot(now.Sub(st.genesis) / (4 * st.cfg.Delta))
}

func (st *Store) voteTime(s primitives.Slot) time.Time { return st.slotStart(s).Add(st.cfg.Delta) }
func (st *Store) confirmationCutoff(s primitives.Slot) time.Time {
	return st.slotStart(s).Add(2 * st.cfg.Delta)
}
func (st *Store) supportCutoff(s primitives.Slot) time.Time {
	return st.slotStart(s).Add(3 * st.cfg.Delta)
}
func (st *Store) unlockTime(s primitives.Slot) time.Time {
	return st.slotStart(s).Add(5 * st.cfg.Delta)
}
func (st *Store) evaluationTime(s primitives.Slot) time.Time {
	return st.slotStart(s).Add(6 * st.cfg.Delta)
}

// Spec: joined, the node has observed since the start of the previous slot. Before that it votes, proposes, unlocks and confirms nothing.
func (st *Store) joined(slot primitives.Slot) bool {
	return slot > 0 && !st.restartTime.After(st.slotStart(slot-1))
}

// Spec: ac_on_tick. Locks expire first, the previous slot is unlocked against at the vote time and evaluated at the confirmation cutoff, each once. Call it before processing objects received at the same instant.
func (st *Store) OnTick(now time.Time) {
	s := st.slotAt(now)
	kept := st.locks[:0]
	for _, l := range st.locks {
		if st.live(l, now) {
			kept = append(kept, l)
		}
	}
	st.locks = kept
	if s == 0 {
		return
	}
	if !now.Before(st.voteTime(s)) && st.unlockedThrough < s && st.joined(s) {
		st.unlockedThrough = s
		st.unlock(s-1, now)
	}
	if !now.Before(st.confirmationCutoff(s)) && st.evaluatedThrough < s {
		st.evaluatedThrough = s
		st.updateConfirmation(s-1, now)
	}
}

// Spec: ac_restart. Every retained block is restamped at now and its carried votes replayed in parent order into fresh pools, saved confirmations and the fast set are cleared, locks keep their deadlines and the confirmation record stays. Observation counts from now.
func (st *Store) Restart(now time.Time) {
	st.restartTime = now
	st.pool = make(map[primitives.Slot]*bucket)
	st.saved = make(map[primitives.Slot]*Node)
	st.fastConfirmed = st.fastConfirmed[:0]
	stack := []*Node{st.root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n.processed = now
		for _, v := range n.BlockVotes {
			st.addVote(v, now)
		}
		stack = append(stack, n.Children...)
	}
}

// Slot s-1 is still read by the slot-s vote and by the slot-(s-1) confirmation, older slots are not. Saved confirmations stay for the slow lock test.
func (st *Store) NewSlot(slot primitives.Slot) {
	for k := range st.pool {
		if k+1 < slot {
			delete(st.pool, k)
		}
	}
	for k := range st.saved {
		if k+primitives.Slot(st.cfg.LockStreak) < slot {
			delete(st.saved, k)
		}
	}
}
