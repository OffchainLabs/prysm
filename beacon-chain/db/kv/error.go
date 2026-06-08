package kv

import "github.com/pkg/errors"

// ErrDeleteJustifiedAndFinalized is raised when we attempt to delete a finalized block/state
var ErrDeleteJustifiedAndFinalized = errors.New("cannot delete finalized block or state")

// ErrNotFound can be used directly, or as a wrapped DBError, whenever a db method needs to
// indicate that a value couldn't be found.
var ErrNotFound = errors.New("not found in db")
var ErrNotFoundState = errors.Wrap(ErrNotFound, "state not found")

// ErrNotFoundOriginBlockRoot is an error specifically for the origin block root getter
var ErrNotFoundOriginBlockRoot = errors.Wrap(ErrNotFound, "OriginBlockRoot")

// ErrNotFoundGenesisBlockRoot means no genesis block root was found, indicating the db was not initialized with genesis
var ErrNotFoundGenesisBlockRoot = errors.Wrap(ErrNotFound, "OriginGenesisRoot")

// ErrNotFoundFeeRecipient is a not found error specifically for the fee recipient getter
var ErrNotFoundFeeRecipient = errors.Wrap(ErrNotFound, "fee recipient")

// ErrNotFoundMetadataSeqNum is a not found error specifically for the metadata sequence number getter
var ErrNotFoundMetadataSeqNum = errors.Wrap(ErrNotFound, "metadata sequence number")

// ErrStateDiffIncompatible is returned when state-diff feature is enabled
// but the database was created without state-diff support.
var ErrStateDiffIncompatible = errors.New("state-diff feature enabled but database was created without state-diff support")

// ErrStateDiffCorrupted is returned when state-diff metadata or data is missing or invalid.
var ErrStateDiffCorrupted = errors.New("state-diff database corrupted")

// ErrStateDiffExponentMismatch is returned when configured exponents differ from stored metadata.
var ErrStateDiffExponentMismatch = errors.New("state-diff exponents mismatch")

// ErrStateDiffMissingSnapshot is returned when the offset snapshot is missing.
var ErrStateDiffMissingSnapshot = errors.New("state-diff offset snapshot missing")

var errEmptyBlockSlice = errors.New("[]blocks.ROBlock is empty")
var errIncorrectBlockParent = errors.New("unexpected missing or forked blocks in a []ROBlock")
var errFinalizedChildNotFound = errors.New("unable to find finalized root descending from backfill batch")
var errNotConnectedToFinalized = errors.New("unable to finalize backfill blocks, finalized parent_root does not match")
