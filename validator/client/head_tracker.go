package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
)

// headTracker holds the latest head (block root, its slot and its payload
// status) the validator has learned about from beacon-node head events.
type headTracker struct {
	mu sync.RWMutex

	head iface.Head
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
func (h *headTracker) update(slot primitives.Slot, blockRoot string, payloadStatus api.PayloadStatus) error {
	// The gRPC client announces no block root, leaving the tracker unset.
	if blockRoot == "" {
		return nil
	}

	root, err := bytesutil.DecodeHex32(blockRoot)
	if err != nil {
		return fmt.Errorf("decode hex 32: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.set && slot < h.head.Slot {
		return nil
	}

	h.head, h.set = iface.Head{Root: root, Slot: slot, PayloadStatus: payloadStatus}, true

	return nil
}

// withHeadHint attaches a freshness hint carrying the latest tracked head. It
// errors if the slot deadline cannot be computed; see withHint.
func (v *validator) withHeadHint(ctx context.Context, slot primitives.Slot, component primitives.BP) (context.Context, error) {
	head := func() (iface.Head, bool) {
		h := v.head

		h.mu.RLock()
		defer h.mu.RUnlock()

		return h.head, h.set
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
	head := func() (iface.Head, bool) {
		root, ok := v.payloadAvailability.payloadRoot(slot)
		// A known payload root is by definition a full payload.
		return iface.Head{Root: root, Slot: slot, PayloadStatus: api.PayloadStatusFull}, ok
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
	head func() (iface.Head, bool),
) (context.Context, error) {
	deadline, err := v.slotComponentDeadline(slot, component)
	if err != nil {
		return ctx, fmt.Errorf("slot component deadline: %w", err)
	}

	return iface.WithHint(ctx, iface.Hint{Head: head, Deadline: deadline}), nil
}
