package decoupled

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// Spec: Δ, k and |K_s|. The slot is 4Δ. ForkSlot is the first slot with votes, blocks before it were chosen by the legacy fork choice.
type Config struct {
	Delta         time.Duration
	LockStreak    uint64
	CommitteeSize uint64
	ForkSlot      primitives.Slot
}

// Spec: GoldfishVote (val_index, slot, head).
type Vote struct {
	Validator primitives.ValidatorIndex
	Slot      primitives.Slot
	Root      [32]byte
}

// Spec: HeightPair.
type Pair struct {
	Height primitives.Height
	Root   [32]byte
}

// The post-state pairs the fork choice reads, target_pair, justified_pair and finalized_pair.
type FGSummary struct {
	Target    Pair
	Justified Pair
	Finalized Pair
}

// Spec: B.votes and B.support_votes, the proposer's lists committed in the block.
type Node struct {
	Root         [32]byte
	Slot         primitives.Slot
	Parent       *Node
	Children     []*Node
	BlockVotes   []Vote
	SupportVotes []Vote
	FG           FGSummary
	processed    time.Time
}

// Spec: A.gf_votes[s] with A.timestamp, at most two distinct votes per validator.
type bucket struct {
	votes   []Vote
	at      []time.Time
	index   map[Vote]int
	votesBy map[primitives.ValidatorIndex]int
}

// Spec: a lock (B, d).
type lock struct {
	block    *Node
	deadline time.Time
}

type Lock struct {
	Root     [32]byte
	Slot     primitives.Slot
	Deadline time.Time
}

// Spec: the FG store Σ and the AC state A, the SG fields are gone.
type Store struct {
	cfg              Config
	genesis          time.Time
	genesisRoot      [32]byte
	restartTime      time.Time
	unlockedThrough  primitives.Slot
	evaluatedThrough primitives.Slot
	root             *Node
	nodes            map[[32]byte]*Node
	bySlot           map[primitives.Slot][]*Node
	finalized        *Node
	justified        *Node
	hJ               primitives.Height
	hF               primitives.Height
	hMax             primitives.Height
	pool             map[primitives.Slot]*bucket
	saved            map[primitives.Slot]*Node
	locks            []lock
	fastConfirmed    []*Node
	latestConfirmed  *Node
}
