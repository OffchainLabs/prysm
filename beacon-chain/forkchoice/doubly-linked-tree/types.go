package doublylinkedtree

import (
	"sync"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// ForkChoice defines the overall fork choice store which includes all block nodes, validator's latest votes and balances.
type ForkChoice struct {
	sync.RWMutex
	store             *Store
	votes             []Vote                      // tracks individual validator's last vote.
	balances          []uint64                    // tracks individual validator's balances last accounted in votes.
	justifiedBalances []uint64                    // tracks individual validator's last justified balances.
	balancesByRoot    forkchoice.BalancesByRooter // handler to obtain balances for the state with a given root
}

var _ forkchoice.ForkChoicer = (*ForkChoice)(nil)

// Store defines the fork choice store which includes block nodes and the last view of checkpoint information.
type Store struct {
	justifiedCheckpoint           *forkchoicetypes.Checkpoint                   // latest justified epoch in store.
	unrealizedJustifiedCheckpoint *forkchoicetypes.Checkpoint                   // best unrealized justified checkpoint in store.
	unrealizedFinalizedCheckpoint *forkchoicetypes.Checkpoint                   // best unrealized finalized checkpoint in store.
	prevJustifiedCheckpoint       *forkchoicetypes.Checkpoint                   // previous justified checkpoint in store.
	finalizedCheckpoint           *forkchoicetypes.Checkpoint                   // latest finalized epoch in store.
	proposerBoostRoot             [fieldparams.RootLength]byte                  // latest block root that was boosted after being received in a timely manner.
	previousProposerBoostRoot     [fieldparams.RootLength]byte                  // previous block root that was boosted after being received in a timely manner.
	previousProposerBoostScore    uint64                                        // previous proposer boosted root score.
	finalizedDependentRoot        [fieldparams.RootLength]byte                  // dependent root at finalized checkpoint.
	finalizedPayloadBlockHash     [fieldparams.RootLength]byte                  // cached payload hash at the finalized checkpoint. Refreshed before pruning at finalization since the node it resolves from is removed by prune.
	committeeWeight               uint64                                        // tracks the total active validator balance divided by the number of slots per Epoch.
	treeRootNode                  *Node                                         // the root node of the store tree.
	headNode                      *Node                                         // last head Node
	emptyNodeByRoot               map[[fieldparams.RootLength]byte]*PayloadNode // nodes indexed by roots.
	fullNodeByRoot                map[[fieldparams.RootLength]byte]*PayloadNode // full nodes (the payload was present) indexed by beacon block root.
	slashedIndices                map[primitives.ValidatorIndex]bool            // the list of equivocating validator indices
	blockRootsBySlotProposer      map[proposerSlotKey][][32]byte                // up to two block roots observed for a (slot, proposer); pruned at finalization.
	originRoot                    [fieldparams.RootLength]byte                  // The genesis block root
	genesisTime                   time.Time
	highestReceivedNode           *Node                                      // The highest slot node.
	receivedBlocksLastEpoch       [fieldparams.SlotsPerEpoch]primitives.Slot // Using `highestReceivedSlot`. The slot of blocks received in the last epoch.
	allTipsAreInvalid             bool                                       // tracks if all tips are not viable for head
}

// Node defines the individual block which includes its block parent, ancestor and how much weight accounted for it.
// This is used as an array based stateful DAG for efficient fork choice look up.
type Node struct {
	slot                        primitives.Slot              // slot of the block converted to the node.
	proposerIndex               primitives.ValidatorIndex    // proposer index of the block.
	root                        [fieldparams.RootLength]byte // root of the block converted to the node.
	blockHash                   [fieldparams.RootLength]byte // payloadHash of the block converted to the node.
	parent                      *PayloadNode                 // parent index of this node.
	target                      *Node                        // target checkpoint for
	bestDescendant              *Node                        // bestDescendant node of this node.
	justifiedEpoch              primitives.Epoch             // justifiedEpoch of this node.
	unrealizedJustifiedEpoch    primitives.Epoch             // the epoch that would be justified if the block would be advanced to the next epoch.
	finalizedEpoch              primitives.Epoch             // finalizedEpoch of this node.
	unrealizedFinalizedEpoch    primitives.Epoch             // the epoch that would be finalized if the block would be advanced to the next epoch.
	balance                     uint64                       // the balance that voted for this node directly
	weight                      uint64                       // weight of this node: the total balance including children
	payloadAvailabilityVote     bitfield.Bitvector512        // PTC payload availability votes
	payloadDataAvailabilityVote bitfield.Bitvector512        // PTC payload data availability votes
}

// PayloadNode defines a full Forkchoice node after the Gloas fork, with the payload status either empty of full
type PayloadNode struct {
	optimistic     bool      // whether the block has been fully validated or not
	full           bool      // whether this node represents a payload present or not
	weight         uint64    // weight of this node: the total balance including children
	balance        uint64    // the balance that voted for this node directly
	gasLimit       uint64    // execution payload gas limit (only set on full nodes).
	bestDescendant *Node     // bestDescendant node of this payload node.
	node           *Node     // the consensus part of this full forkchoice node
	timestamp      time.Time // The timestamp when the node was inserted.
	children       []*Node   // the list of direct children of this Node
}

type proposerSlotKey struct {
	slot     primitives.Slot
	proposer primitives.ValidatorIndex
}

// Vote defines an individual validator's vote.
type Vote struct {
	currentRoot          [fieldparams.RootLength]byte // current voting root.
	nextRoot             [fieldparams.RootLength]byte // next voting root.
	nextSlot             primitives.Slot              // slot of the next voting period.
	currentSlot          primitives.Slot              // slot of the current voting period.
	nextPayloadStatus    bool                         // whether the next vote is for a full or empty payload
	currentPayloadStatus bool                         // whether the current vote is for a full or empty payload
}
