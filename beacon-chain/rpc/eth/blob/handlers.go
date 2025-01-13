package blob

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	field_params "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// Blobs is an HTTP handler for Beacon API getBlobs.
func (s *Server) Blobs(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.Blobs")
	defer span.End()

	indices, err := parseIndices(r.URL, s.TimeFetcher.CurrentSlot())
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}
	segments := strings.Split(r.URL.Path, "/")
	blockId := segments[len(segments)-1]

	verifiedBlobs, rpcErr := s.Blocker.Blobs(ctx, blockId, indices)
	if rpcErr != nil {
		code := core.ErrorReasonToHTTP(rpcErr.Reason)
		switch code {
		case http.StatusBadRequest:
			httputil.HandleError(w, "Invalid block ID: "+rpcErr.Err.Error(), code)
			return
		case http.StatusNotFound:
			httputil.HandleError(w, "Block not found: "+rpcErr.Err.Error(), code)
			return
		case http.StatusInternalServerError:
			httputil.HandleError(w, "Internal server error: "+rpcErr.Err.Error(), code)
			return
		default:
			httputil.HandleError(w, rpcErr.Err.Error(), code)
			return
		}
	}

	if httputil.RespondWithSsz(r) {
		sszResp, err := buildSidecarsSSZResponse(verifiedBlobs)
		if err != nil {
			httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszResp, "blob_sidecars.ssz")
		return
	}

	blk, err := s.Blocker.Block(ctx, []byte(blockId))
	if err != nil {
		httputil.HandleError(w, "Could not fetch block: "+err.Error(), http.StatusInternalServerError)
		return
	}
	blkRoot, err := blk.Block().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not hash block: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, blkRoot)
	if err != nil {
		httputil.HandleError(w, "Could not check if block is optimistic: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := buildSidecarsJsonResponse(verifiedBlobs)
	resp := &structs.SidecarsResponse{
		Version:             version.String(blk.Version()),
		Data:                data,
		ExecutionOptimistic: isOptimistic,
		Finalized:           s.FinalizationFetcher.IsFinalized(ctx, blkRoot),
	}
	httputil.WriteJson(w, resp)
}

// parseIndices filters out invalid and duplicate blob indices
func parseIndices(url *url.URL, s primitives.Slot) ([]uint64, error) {
	rawIndices := url.Query()["indices"]
	indices := make([]uint64, 0, params.BeaconConfig().MaxBlobsPerBlock(s))
	invalidIndices := make([]string, 0)
loop:
	for _, raw := range rawIndices {
		ix, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			invalidIndices = append(invalidIndices, raw)
			continue
		}
		if ix >= uint64(params.BeaconConfig().MaxBlobsPerBlock(s)) {
			invalidIndices = append(invalidIndices, raw)
			continue
		}
		for i := range indices {
			if ix == indices[i] {
				continue loop
			}
		}
		indices = append(indices, ix)
	}

	if len(invalidIndices) > 0 {
		return nil, fmt.Errorf("requested blob indices %v are invalid", invalidIndices)
	}
	return indices, nil
}

func buildSidecarsJsonResponse(verifiedBlobs []*blocks.VerifiedROBlob) []*structs.Sidecar {
	sidecars := make([]*structs.Sidecar, len(verifiedBlobs))
	for i, sc := range verifiedBlobs {
		proofs := make([]string, len(sc.CommitmentInclusionProof))
		for j := range sc.CommitmentInclusionProof {
			proofs[j] = hexutil.Encode(sc.CommitmentInclusionProof[j])
		}
		sidecars[i] = &structs.Sidecar{
			Index:                    strconv.FormatUint(sc.Index, 10),
			Blob:                     hexutil.Encode(sc.Blob),
			KzgCommitment:            hexutil.Encode(sc.KzgCommitment),
			SignedBeaconBlockHeader:  structs.SignedBeaconBlockHeaderFromConsensus(sc.SignedBlockHeader),
			KzgProof:                 hexutil.Encode(sc.KzgProof),
			CommitmentInclusionProof: proofs,
		}
	}
	return sidecars
}

func buildSidecarsSSZResponse(verifiedBlobs []*blocks.VerifiedROBlob) ([]byte, error) {
	ssz := make([]byte, field_params.BlobSidecarSize*len(verifiedBlobs))
	for i, sidecar := range verifiedBlobs {
		sszrep, err := sidecar.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal sidecar ssz")
		}
		copy(ssz[i*field_params.BlobSidecarSize:(i+1)*field_params.BlobSidecarSize], sszrep)
	}
	return ssz, nil
}
