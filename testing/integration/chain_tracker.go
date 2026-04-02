package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// chainTracker subscribes to SSE block events on multiple beacon nodes and
// prints a live chain visualization as blocks arrive.
type chainTracker struct {
	t  *testing.T
	mu sync.Mutex
	// blocks[nodeIndex] = sorted list of slots seen
	blocks map[int][]uint64
	// finalized[nodeIndex] = highest finalized epoch
	finalized map[int]uint64
	// reorgs[nodeIndex] = reorg count
	reorgs        map[int]int
	slotsPerEpoch uint64
}

func newChainTracker(t *testing.T, slotsPerEpoch uint64) *chainTracker {
	return &chainTracker{
		t:             t,
		blocks:        make(map[int][]uint64),
		finalized:     make(map[int]uint64),
		reorgs:        make(map[int]int),
		slotsPerEpoch: slotsPerEpoch,
	}
}

// track starts a background SSE listener for the given beacon node.
// It returns immediately. Cancel ctx to stop.
func (ct *chainTracker) track(ctx context.Context, nodeIndex int) {
	go ct.listenSSE(ctx, nodeIndex)
}

// render returns the current chain visualization for all tracked nodes.
func (ct *chainTracker) render() string {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	// Collect node indices and sort them.
	var indices []int
	for idx := range ct.blocks {
		indices = append(indices, idx)
	}
	slices.Sort(indices)

	var sb strings.Builder
	for _, idx := range indices {
		slots := ct.blocks[idx]
		fin := ct.finalized[idx]
		reorgs := ct.reorgs[idx]
		sb.WriteString(fmt.Sprintf("  beacon-%d: %s", idx, ct.renderChain(slots, fin)))
		if reorgs > 0 {
			sb.WriteString(fmt.Sprintf(" ⚠ %d reorg(s)", reorgs))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderChain builds a compact chain string like: [1]←[2]←[3]←...←[22]←[23]←[24] | finalized: epoch 3
func (ct *chainTracker) renderChain(slots []uint64, finalizedEpoch uint64) string {
	if len(slots) == 0 {
		return "(empty)"
	}

	var sb strings.Builder
	maxShow := 12 // Show at most this many blocks in the visualization.
	if len(slots) <= maxShow {
		for i, s := range slots {
			if i > 0 {
				sb.WriteString("←")
			}
			sb.WriteString(ct.slotTag(s, finalizedEpoch))
		}
	} else {
		// Show first few, ellipsis, last few.
		half := maxShow / 2
		for i := range half {
			if i > 0 {
				sb.WriteString("←")
			}
			sb.WriteString(ct.slotTag(slots[i], finalizedEpoch))
		}
		sb.WriteString("←...←")
		for i := len(slots) - half; i < len(slots); i++ {
			if i > len(slots)-half {
				sb.WriteString("←")
			}
			sb.WriteString(ct.slotTag(slots[i], finalizedEpoch))
		}
	}

	sb.WriteString(fmt.Sprintf(" | finalized: epoch %d", finalizedEpoch))
	return sb.String()
}

// slotTag renders a single slot, marking epoch boundaries.
func (ct *chainTracker) slotTag(slot uint64, finalizedEpoch uint64) string {
	finSlot := finalizedEpoch * ct.slotsPerEpoch
	if slot <= finSlot {
		return fmt.Sprintf("[%d]", slot)
	}
	return fmt.Sprintf("(%d)", slot)
}

func (ct *chainTracker) addBlock(nodeIndex int, slot uint64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	slots := ct.blocks[nodeIndex]
	// Insert in sorted order (blocks usually arrive in order).
	if len(slots) == 0 || slot > slots[len(slots)-1] {
		ct.blocks[nodeIndex] = append(slots, slot)
	} else {
		if slices.Contains(slots, slot) {
			return
		}
		ct.blocks[nodeIndex] = append(slots, slot)
		slices.Sort(ct.blocks[nodeIndex])
	}
}

func (ct *chainTracker) setFinalized(nodeIndex int, epoch uint64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if epoch > ct.finalized[nodeIndex] {
		ct.finalized[nodeIndex] = epoch
	}
}

func (ct *chainTracker) addReorg(nodeIndex int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.reorgs[nodeIndex]++
}

func (ct *chainTracker) getFinalized(nodeIndex int) uint64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.finalized[nodeIndex]
}

func (ct *chainTracker) getReorgCount(nodeIndex int) int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.reorgs[nodeIndex]
}

func (ct *chainTracker) blockCount(nodeIndex int) int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return len(ct.blocks[nodeIndex])
}

func (ct *chainTracker) listenSSE(ctx context.Context, nodeIndex int) {
	url := fmt.Sprintf("http://127.0.0.1:%d/eth/v1/events?topics=block,finalized_checkpoint,chain_reorg",
		beaconGRPCPort(nodeIndex))

	for ctx.Err() == nil {
		ct.sseLoop(ctx, url, nodeIndex)
		if ctx.Err() != nil {
			return
		}
	}
}

func (ct *chainTracker) sseLoop(ctx context.Context, url string, nodeIndex int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()

		if after, ok := strings.CutPrefix(line, "event:"); ok {
			currentEvent = strings.TrimSpace(after)
			continue
		}
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)

		switch currentEvent {
		case "block":
			var ev struct {
				Slot string `json:"slot"`
			}
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			slot, _ := strconv.ParseUint(ev.Slot, 10, 64)
			ct.addBlock(nodeIndex, slot)

		case "finalized_checkpoint":
			var ev struct {
				Epoch string `json:"epoch"`
			}
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			epoch, _ := strconv.ParseUint(ev.Epoch, 10, 64)
			ct.setFinalized(nodeIndex, epoch)
			ct.t.Logf("beacon-%d finalized epoch %d (%d blocks)", nodeIndex, epoch, ct.blockCount(nodeIndex))

		case "chain_reorg":
			ct.addReorg(nodeIndex)
			ct.t.Logf("beacon-%d REORG (total: %d)", nodeIndex, ct.getReorgCount(nodeIndex))
		}
	}
}
