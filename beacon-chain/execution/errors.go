package execution

import "github.com/pkg/errors"

var (
	// ErrUnknownPayloadStatus when the payload status is unknown.
	ErrUnknownPayloadStatus = errors.New("unknown payload status")
	// ErrAcceptedSyncingPayloadStatus when the status of the payload is syncing or accepted.
	ErrAcceptedSyncingPayloadStatus = errors.New("payload status is SYNCING or ACCEPTED")
	// ErrInvalidPayloadStatus when the status of the payload is invalid.
	ErrInvalidPayloadStatus = errors.New("payload status is INVALID")
	// ErrInvalidBlockHashPayloadStatus when the status of the payload fails to validate block hash.
	ErrInvalidBlockHashPayloadStatus = errors.New("payload status is INVALID_BLOCK_HASH")
	// ErrNilResponse when the response is nil.
	ErrNilResponse = errors.New("nil response")
	// ErrUnsupportedVersion represents a case where a payload is requested for a block type that doesn't have a known mapping.
	ErrUnsupportedVersion = errors.New("unknown ExecutionPayload schema for block version")
)
