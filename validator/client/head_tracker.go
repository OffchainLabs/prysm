package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
)

// headTracker holds the latest head (block root and its slot) the validator has
// learned about from beacon-node head events.
type headTracker struct {
	mu sync.RWMutex

	slot primitives.Slot
	root [32]byte
	set  bool
}

func newHeadTracker() *headTracker {
	return &headTracker{}
}

// update records blockRoot as the expected head, keeping the highest slot.
//
// A reorg moving the head backwards in slot is dropped, pinning a root no node
// will report again. This is benign: the freshness criterion then never matches,
// so the read falls back to the freshest response at its deadline, and the next
// event at a slot >= the pinned one heals the tracker.
func (h *headTracker) update(slot primitives.Slot, blockRoot string) error {
	if blockRoot == "" {
		return nil
	}

	root, err := bytesutil.DecodeHex32(blockRoot)
	if err != nil {
		return fmt.Errorf("decode hex 32: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.set && slot < h.slot {
		return nil
	}

	h.slot, h.root, h.set = slot, root, true

	return nil
}

// withHeadHint attaches a freshness hint carrying the latest tracked head. It
// errors if the slot deadline cannot be computed; see withHint.
func (v *validator) withHeadHint(ctx context.Context, slot primitives.Slot, component primitives.BP) (context.Context, error) {
	head := func() ([32]byte, primitives.Slot, bool) {
		h := v.head

		h.mu.RLock()
		defer h.mu.RUnlock()

		return h.root, h.slot, h.set
	}

	hint, err := v.withHint(ctx, slot, component, head)
	if err != nil {
		return ctx, fmt.Errorf("with hint: %w", err)
	}

	return hint, nil
}

// withPayloadHeadHint attaches a freshness hint carrying the payload block root
// known for slot.
func (v *validator) withPayloadHeadHint(ctx context.Context, slot primitives.Slot) (context.Context, error) {
	head := func() ([32]byte, primitives.Slot, bool) {
		root, ok := v.payloadAvailability.payloadRoot(slot)
		return root, slot, ok
	}

	hint, err := v.withHint(ctx, slot, params.BeaconConfig().PayloadAttestationDueBPS, head)
	if err != nil {
		return ctx, fmt.Errorf("with hint: %w", err)
	}

	return hint, nil
}

// withHint attaches a freshness hint (head resolver plus the slot/component
// deadline) to ctx. It returns the unmodified ctx and an error if the deadline
// cannot be computed (a slot overflow), leaving it to the caller to decide
// whether to proceed without a hint or abort.
func (v *validator) withHint(
	ctx context.Context,
	slot primitives.Slot,
	component primitives.BP,
	head func() (root [32]byte, slot primitives.Slot, ok bool),
) (context.Context, error) {
	deadline, err := v.slotComponentDeadline(slot, component)
	if err != nil {
		return ctx, fmt.Errorf("slot component deadline: %w", err)
	}

	return iface.WithHint(ctx, iface.Hint{Head: head, Deadline: deadline}), nil
}
