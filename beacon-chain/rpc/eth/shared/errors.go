package shared

import (
	"net/http"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/lookup"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
)

// WriteStateFetchError writes an appropriate error based on the supplied argument.
// The argument error should be a result of fetching state.
func WriteStateFetchError(w http.ResponseWriter, err error) {
	var stateNotFoundError *lookup.StateNotFoundError
	if errors.As(err, &stateNotFoundError) {
		httputil.HandleError(w, "State not found", http.StatusNotFound)
		return
	}
	var parseErr *lookup.StateIdParseError
	if errors.As(err, &parseErr) {
		httputil.HandleError(w, "Invalid state ID: "+parseErr.Error(), http.StatusBadRequest)
		return
	}
	httputil.HandleError(w, "Could not get state: "+err.Error(), http.StatusInternalServerError)
}

// WriteBlockFetchError writes an appropriate error based on the supplied argument.
// The argument error should be a result of fetching block.
func WriteBlockFetchError(w http.ResponseWriter, blk interfaces.ReadOnlySignedBeaconBlock, err error) bool {
	var invalidBlockIdErr *lookup.BlockIdParseError
	if errors.As(err, &invalidBlockIdErr) {
		httputil.HandleError(w, "Invalid block ID: "+invalidBlockIdErr.Error(), http.StatusBadRequest)
		return false
	}
	if err != nil {
		httputil.HandleError(w, "Could not get block from block ID: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	if err = blocks.BeaconBlockIsNil(blk); err != nil {
		httputil.HandleError(w, "Could not find requested block: "+err.Error(), http.StatusNotFound)
		return false
	}
	return true
}
