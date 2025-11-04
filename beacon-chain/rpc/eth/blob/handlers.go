package blob

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/options"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// Blobs is an HTTP handler for Beacon API getBlobs.
// Deprecated: /eth/v1/beacon/blob_sidecars/{block_id} in favor of /eth/v1/beacon/blobs/{block_id}
// the endpoint will continue to work post fulu for some time however
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

	verifiedBlobs, rpcErr := s.Blocker.Blobs(ctx, blockId, options.WithIndices(indices))
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

	blk, err := s.Blocker.Block(ctx, []byte(blockId))
	if !shared.WriteBlockFetchError(w, blk, err) {
		return
	}

	if httputil.RespondWithSsz(r) {
		sszResp, err := buildSidecarsSSZResponse(verifiedBlobs)
		if err != nil {
			httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(api.VersionHeader, version.String(blk.Version()))
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
	w.Header().Set(api.VersionHeader, version.String(blk.Version()))
	httputil.WriteJson(w, resp)
}

// parseIndices filters out invalid and duplicate blob indices
func parseIndices(url *url.URL, s primitives.Slot) ([]int, error) {
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(s)
	rawIndices := url.Query()["indices"]
	indices := make([]int, 0, maxBlobsPerBlock)
	invalidIndices := make([]string, 0)
loop:
	for _, raw := range rawIndices {
		ix, err := strconv.Atoi(raw)
		if err != nil {
			invalidIndices = append(invalidIndices, raw)
			continue
		}
		if !(0 <= ix && ix < maxBlobsPerBlock) {
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

// GetBlobs retrieves blobs for a given block id. ( this is the new handler that replaces func (s *Server) Blobs )
func (s *Server) GetBlobs(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetBlobs")
	defer span.End()

	segments := strings.Split(r.URL.Path, "/")
	blockId := segments[len(segments)-1]

	var verifiedBlobs []*blocks.VerifiedROBlob
	var rpcErr *core.RpcError

	// Check if versioned_hashes parameter is provided
	versionedHashesStr := r.URL.Query()["versioned_hashes"]
	versionedHashes := make([][]byte, len(versionedHashesStr))
	if len(versionedHashesStr) > 0 {
		for i, hashStr := range versionedHashesStr {
			hash, ok := shared.ValidateHex(w, fmt.Sprintf("versioned_hashes[%d]", i), hashStr, 32)
			if !ok {
				return
			}
			versionedHashes[i] = hash
		}
	}
	verifiedBlobs, rpcErr = s.Blocker.Blobs(ctx, blockId, options.WithVersionedHashes(versionedHashes))
	if rpcErr != nil {
		code := core.ErrorReasonToHTTP(rpcErr.Reason)
		switch code {
		case http.StatusBadRequest:
			httputil.HandleError(w, "Bad Request: "+rpcErr.Err.Error(), code)
			return
		case http.StatusNotFound:
			httputil.HandleError(w, "Not found: "+rpcErr.Err.Error(), code)
			return
		case http.StatusInternalServerError:
			httputil.HandleError(w, "Internal server error: "+rpcErr.Err.Error(), code)
			return
		default:
			httputil.HandleError(w, rpcErr.Err.Error(), code)
			return
		}
	}

	blk, err := s.Blocker.Block(ctx, []byte(blockId))
	if !shared.WriteBlockFetchError(w, blk, err) {
		return
	}

	if httputil.RespondWithSsz(r) {
		sszLen := fieldparams.BlobSize
		sszData := make([]byte, len(verifiedBlobs)*sszLen)
		for i := range verifiedBlobs {
			copy(sszData[i*sszLen:(i+1)*sszLen], verifiedBlobs[i].Blob)
		}

		w.Header().Set(api.VersionHeader, version.String(blk.Version()))
		httputil.WriteSsz(w, sszData)
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

	data := make([]string, len(verifiedBlobs))
	for i, v := range verifiedBlobs {
		data[i] = hexutil.Encode(v.Blob)
	}
	resp := &structs.GetBlobsResponse{
		Data:                data,
		ExecutionOptimistic: isOptimistic,
		Finalized:           s.FinalizationFetcher.IsFinalized(ctx, blkRoot),
	}
	w.Header().Set(api.VersionHeader, version.String(blk.Version()))
	httputil.WriteJson(w, resp)
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
	ssz := make([]byte, fieldparams.BlobSidecarSize*len(verifiedBlobs))
	for i, sidecar := range verifiedBlobs {
		sszrep, err := sidecar.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal sidecar ssz")
		}
		copy(ssz[i*fieldparams.BlobSidecarSize:(i+1)*fieldparams.BlobSidecarSize], sszrep)
	}
	return ssz, nil
}
