package primitives

// Height is the finality gadget counter. It advances only when a height outcome is consumed, never with the clock.
type Height uint64
