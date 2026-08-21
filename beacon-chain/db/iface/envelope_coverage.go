package iface

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

// ErrSlotIndexCursorInvalidated is returned by the block slot index page
// accessors when an in-process resume token no longer matches the stored
// packed value (for example because an interleaved delete compacted the value
// at or before the token offset). The caller must restart the current slot
// from its beginning.
var ErrSlotIndexCursorInvalidated = errors.New("block slot index cursor invalidated")

// ErrEnvelopeCoveragePrimaryMismatch is returned by CommitEnvelopeCoverage
// when the stored primary envelope value for an index entry is missing or no
// longer matches the fingerprint validated outside the transaction. The whole
// coverage publication is aborted.
var ErrEnvelopeCoveragePrimaryMismatch = errors.New("stored execution payload envelope diverged from validated value")

// RevealedEnvelopeSlotRoot pairs a slot with the beacon block root whose
// envelope the canonical chain actually used at that slot.
type RevealedEnvelopeSlotRoot struct {
	Slot primitives.Slot
	Root [32]byte
}

// RevealedEnvelopeIndexEntry is one canonical-revealed serving index put
// prepared by the coverage coordinator. PrimaryFingerprint is the SHA-256 of
// the raw stored primary envelope value at the time the coordinator validated
// it; the combined commit rechecks it inside the write transaction.
type RevealedEnvelopeIndexEntry struct {
	Slot               primitives.Slot
	Root               [32]byte
	PrimaryFingerprint [32]byte
}

// EnvelopeIndexReplacement is an exact range replacement of the
// canonical-revealed envelope slot index: every existing key in
// [Start, End) is deleted, then exactly Entries are written. All entries must
// lie inside the range.
type EnvelopeIndexReplacement struct {
	Start   primitives.Slot // inclusive
	End     primitives.Slot // exclusive
	Entries []RevealedEnvelopeIndexEntry
}

// SlotIndexCursor is an ephemeral, in-process-only resume position for the
// block slot index page accessors. It is deliberately never persisted: the
// durable resume point for coverage work is the committed coverage bound, and
// a process restart re-examines a partially examined slot in full.
//
// Slot is the next slot boundary to examine (inclusive). When NextByteOffset
// is non-zero the accessor resumes inside Slot's packed root list at that
// byte offset after validating that the 32 bytes ending at the offset still
// equal LastRoot; a mismatch returns ErrSlotIndexCursorInvalidated.
type SlotIndexCursor struct {
	Slot           primitives.Slot
	NextByteOffset uint64
	LastRoot       [32]byte
}

// SlotIndexCandidate is one (slot, root) candidate copied out of the block
// slot index.
type SlotIndexCandidate struct {
	Slot primitives.Slot
	Root [32]byte
}

// SlotIndexPage is one hard candidate/byte-budgeted page of block slot index
// candidates. Next is nil when the scan bound was reached; otherwise it is
// the cursor for the next call and may point mid-slot when the budget was
// exhausted inside one packed value.
type SlotIndexPage struct {
	Candidates []SlotIndexCandidate
	Next       *SlotIndexCursor
}
