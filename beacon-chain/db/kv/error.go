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

var errEmptyBlockSlice = errors.New("[]blocks.ROBlock is empty")
var errIncorrectBlockParent = errors.New("unexpected missing or forked blocks in a []ROBlock")
var errFinalizedChildNotFound = errors.New("unable to find finalized root descending from backfill batch")
var errNotConnectedToFinalized = errors.New("unable to finalize backfill blocks, finalized parent_root does not match")
