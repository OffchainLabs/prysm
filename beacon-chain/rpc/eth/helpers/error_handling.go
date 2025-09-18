package helpers

import (
	"errors"
	"net/http"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PrepareStateFetchGRPCError returns an appropriate gRPC error based on the supplied argument.
// The argument error should be a result of fetching state.
func PrepareStateFetchGRPCError(err error) error {
	if errors.Is(err, stategen.ErrNoDataForSlot) {
		return status.Errorf(codes.NotFound, "lacking historical data needed to fulfill request")
	}
	var stateNotFoundErr *lookup.StateNotFoundError
	if errors.As(err, &stateNotFoundErr) {
		return status.Errorf(codes.NotFound, "State not found: %v", stateNotFoundErr)
	}
	var parseErr *lookup.StateIdParseError
	if errors.As(err, &parseErr) {
		return status.Errorf(codes.InvalidArgument, "Invalid state ID: %v", parseErr)
	}
	return status.Errorf(codes.Internal, "Invalid state ID: %v", err)
}

// HandleIsOptimisticError handles errors from IsOptimistic function calls and writes appropriate HTTP responses.
func HandleIsOptimisticError(w http.ResponseWriter, err error) {
	var fetchErr *lookup.FetchStateError
	if errors.As(err, &fetchErr) {
		shared.WriteStateFetchError(w, err)
		return
	}

	var blockNotFoundErr *lookup.BlockNotFoundError
	if errors.As(err, &blockNotFoundErr) {
		httputil.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusNotFound)
		return
	}
	httputil.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
}

// IndexedVerificationFailure represents a collection of verification failures.
type IndexedVerificationFailure struct {
	Failures []*SingleIndexedVerificationFailure `json:"failures"`
}

// SingleIndexedVerificationFailure represents an issue when verifying a single indexed object e.g. an item in an array.
type SingleIndexedVerificationFailure struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

func HandleGetBlockError(blk interfaces.ReadOnlySignedBeaconBlock, err error) error {
	var invalidBlockIdErr *lookup.BlockIdParseError
	if errors.As(err, &invalidBlockIdErr) {
		return status.Errorf(codes.InvalidArgument, "Invalid block ID: %v", invalidBlockIdErr)
	}
	if err != nil {
		return status.Errorf(codes.Internal, "Could not get block from block ID: %v", err)
	}
	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		return status.Errorf(codes.NotFound, "Could not find requested block: %v", err)
	}
	return nil
}
