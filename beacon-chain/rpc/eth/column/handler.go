package column

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/blob"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// Server is the HTTP server for handling requests related to data column sidecars.
type Server struct {
	Blocker               lookup.Blocker
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	FinalizationFetcher   blockchain.FinalizationFetcher
	TimeFetcher           blockchain.TimeFetcher
}

// DataColumnSidecars handles requests for data column sidecars associated with a specific block ID.
func (s *Server) DataColumnSidecars(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.DataColumnSidecars")
	defer span.End()

	indices, err := blob.ParseIndices(r.URL, s.TimeFetcher.CurrentSlot())
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	segments := strings.Split(r.URL.Path, "/")
	blockID := segments[len(segments)-1]

	sidecars, rpcErr := s.Blocker.DataColumnSidecars(ctx, blockID, indices)
	if rpcErr != nil {
		code := core.ErrorReasonToHTTP(rpcErr.Reason)
		msg := rpcErr.Err.Error()
		switch code {
		case http.StatusBadRequest:
			httputil.HandleError(w, "Invalid block ID: "+msg, code)
		case http.StatusNotFound:
			httputil.HandleError(w, "Block not found: "+msg, code)
		case http.StatusInternalServerError:
			httputil.HandleError(w, "Internal server error: "+msg, code)
		default:
			httputil.HandleError(w, msg, code)
		}
		return
	}

	blk, err := s.Blocker.Block(ctx, []byte(blockID))
	if err != nil {
		httputil.HandleError(w, "Could not fetch block: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if blk == nil {
		httputil.HandleError(w, "Block not found", http.StatusNotFound)
		return
	}

	versionStr := version.String(blk.Version())
	w.Header().Set(api.VersionHeader, versionStr)

	if httputil.RespondWithSsz(r) {
		sszResp, err := buildColumnsSszResponse(sidecars)
		if err != nil {
			httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszResp)
		return
	}

	blkRoot, err := blk.Block().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not hash block: "+err.Error(), http.StatusInternalServerError)
		return
	}

	isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, blkRoot)
	if err != nil {
		httputil.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := &structs.DataColumnSidecarResponse{
		Version:             versionStr,
		Data:                buildColumnJSONResponse(sidecars),
		ExecutionOptimistic: isOptimistic,
		Finalized:           s.FinalizationFetcher.IsFinalized(ctx, blkRoot),
	}
	httputil.WriteJson(w, resp)
}

func buildColumnsSszResponse(columns []blocks.VerifiedRODataColumn) ([]byte, error) {
	buf := make([]byte, 0)
	for _, col := range columns {
		b, err := col.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "marshal sidecar ssz")
		}
		buf = append(buf, b...)
	}
	return buf, nil
}

func buildColumnJSONResponse(columns []blocks.VerifiedRODataColumn) []*structs.DataColumnSidecar {
	out := make([]*structs.DataColumnSidecar, len(columns))
	for i, col := range columns {
		column := encodeByteSlices(col.Column)
		kzgCommitments := encodeByteSlices(col.KzgCommitments)
		kzgProofs := encodeByteSlices(col.KzgProofs)
		inclusionProofs := encodeByteSlices(col.KzgCommitmentsInclusionProof)

		out[i] = &structs.DataColumnSidecar{
			Index:                        strconv.FormatUint(col.Index, 10),
			Column:                       column,
			KZGCommitments:               kzgCommitments,
			KZGProofs:                    kzgProofs,
			SignedBlockHeader:            structs.SignedBeaconBlockHeaderFromConsensus(col.SignedBlockHeader),
			KZGCommitmentsInclusionProof: inclusionProofs,
		}
	}
	return out
}

func encodeByteSlices(items [][]byte) []string {
	out := make([]string, len(items))
	for i := range items {
		out[i] = hexutil.Encode(items[i])
	}
	return out
}
