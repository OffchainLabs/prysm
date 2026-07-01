package beacon

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	sszquerypb "github.com/OffchainLabs/prysm/v7/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	ssz "github.com/prysmaticlabs/fastssz"
)

// QueryBeaconState handles SSZ Query request for BeaconState.
// Returns as bytes serialized SSZQueryResponse.
func (s *Server) QueryBeaconState(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.QueryBeaconState")
	defer span.End()

	stateID := r.PathValue("state_id")
	if stateID == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}

	// Validate path before lookup: it might be expensive.
	var req structs.SSZQueryRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Query) == 0 {
		httputil.HandleError(w, "Empty query submitted", http.StatusBadRequest)
		return
	}

	path, err := query.ParsePath(req.Query)
	if err != nil {
		httputil.HandleError(w, "Could not parse path '"+req.Query+"': "+err.Error(), http.StatusBadRequest)
		return
	}

	stateRoot, err := s.Stater.StateRoot(ctx, []byte(stateID))
	if err != nil {
		var rootNotFoundErr *lookup.StateRootNotFoundError
		if errors.As(err, &rootNotFoundErr) {
			httputil.HandleError(w, "State root not found: "+rootNotFoundErr.Error(), http.StatusNotFound)
			return
		}
		httputil.HandleError(w, "Could not get state root: "+err.Error(), http.StatusInternalServerError)
		return
	}

	st, err := s.Stater.State(ctx, []byte(stateID))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	// NOTE: Using unsafe conversion to proto is acceptable here,
	// as we play with a copy of the state returned by Stater.
	sszObject, ok := st.ToProtoUnsafe().(query.SSZObject)
	if !ok {
		httputil.HandleError(w, "Unsupported state version for querying: "+version.String(st.Version()), http.StatusBadRequest)
		return
	}

	info, err := query.AnalyzeObject(sszObject)
	if err != nil {
		httputil.HandleError(w, "Could not analyze state object: "+err.Error(), http.StatusInternalServerError)
		return
	}

	finalInfo, offset, length, err := query.CalculateOffsetAndLength(info, path)
	if err != nil {
		httputil.HandleError(w, "Could not calculate offset and length for path '"+req.Query+"': "+err.Error(), http.StatusInternalServerError)
		return
	}

	var result []byte
	if path.Length {
		n, err := finalInfo.LengthValue()
		if err != nil {
			httputil.HandleError(w, "Invalid query '"+req.Query+"': "+err.Error(), http.StatusBadRequest)
			return
		}
		result = binary.LittleEndian.AppendUint64(nil, n)
	} else {
		encodedState, err := st.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "Could not marshal state to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		result = encodedState[offset : offset+length]
	}

	var response ssz.Marshaler
	if req.IncludeProof {
		proof, err := getBeaconStateProof(ctx, st, info, path)
		if err != nil {
			httputil.HandleError(w, "Could not compute merkle proofs for path "+req.Query+": "+err.Error(), http.StatusInternalServerError)
			return
		}
		response = &sszquerypb.SSZQueryResponseWithProof{
			Root:   stateRoot,
			Result: result,
			Proof:  proof,
		}
	} else {
		response = &sszquerypb.SSZQueryResponse{
			Root:   stateRoot,
			Result: result,
		}
	}

	responseSsz, err := response.MarshalSSZ()
	if err != nil {
		httputil.HandleError(w, "Could not marshal response to SSZ: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(api.VersionHeader, version.String(st.Version()))
	httputil.WriteSsz(w, responseSsz)
}

// QueryBeaconBlock handles SSZ Query request for BeaconBlock.
// Returns as bytes serialized SSZQueryResponse.
func (s *Server) QueryBeaconBlock(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.QueryBeaconBlock")
	defer span.End()

	blockId := r.PathValue("block_id")
	if blockId == "" {
		httputil.HandleError(w, "block_id is required in URL params", http.StatusBadRequest)
		return
	}

	// Validate path before lookup: it might be expensive.
	var req structs.SSZQueryRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Query) == 0 {
		httputil.HandleError(w, "Empty query submitted", http.StatusBadRequest)
		return
	}

	path, err := query.ParsePath(req.Query)
	if err != nil {
		httputil.HandleError(w, "Could not parse path '"+req.Query+"': "+err.Error(), http.StatusBadRequest)
		return
	}

	signedBlock, err := s.Blocker.Block(ctx, []byte(blockId))
	if !shared.WriteBlockFetchError(w, signedBlock, err) {
		return
	}

	protoBlock, err := signedBlock.Block().Proto()
	if err != nil {
		httputil.HandleError(w, "Could not convert block to proto: "+err.Error(), http.StatusInternalServerError)
		return
	}

	block, ok := protoBlock.(query.SSZObject)
	if !ok {
		httputil.HandleError(w, "Unsupported block version for querying: "+version.String(signedBlock.Version()), http.StatusBadRequest)
		return
	}

	info, err := query.AnalyzeObject(block)
	if err != nil {
		httputil.HandleError(w, "Could not analyze block object: "+err.Error(), http.StatusInternalServerError)
		return
	}

	finalInfo, offset, length, err := query.CalculateOffsetAndLength(info, path)
	if err != nil {
		httputil.HandleError(w, "Could not calculate offset and length for path '"+req.Query+"': "+err.Error(), http.StatusInternalServerError)
		return
	}

	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not compute block root: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var result []byte
	if path.Length {
		n, err := finalInfo.LengthValue()
		if err != nil {
			httputil.HandleError(w, "Invalid query '"+req.Query+"': "+err.Error(), http.StatusBadRequest)
			return
		}
		result = binary.LittleEndian.AppendUint64(nil, n)
	} else {
		encodedBlock, err := signedBlock.Block().MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "Could not marshal block to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		result = encodedBlock[offset : offset+length]
	}

	var response ssz.Marshaler
	if req.IncludeProof {
		proof, err := getSSZQueryProof(info, path)
		if err != nil {
			httputil.HandleError(w, "Could not compute merkle proofs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		response = &sszquerypb.SSZQueryResponseWithProof{
			Root:   blockRoot[:],
			Result: result,
			Proof: &sszquerypb.SSZQueryProof{
				Leaf:   proof.Leaf,
				Gindex: uint64(proof.Index),
				Proofs: proof.Hashes,
			},
		}
	} else {
		response = &sszquerypb.SSZQueryResponse{
			Root:   blockRoot[:],
			Result: result,
		}
	}
	responseSsz, err := response.MarshalSSZ()
	if err != nil {
		httputil.HandleError(w, "Could not marshal response to SSZ: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(signedBlock.Version()))
	httputil.WriteSsz(w, responseSsz)
}

// getSSZQueryProof retrieves Merkle proof for a given SSZInfo object and query path
func getSSZQueryProof(info *query.SszInfo, path query.Path) (*ssz.Proof, error) {
	gi, err := query.GetGeneralizedIndexFromPath(info, path)
	if err != nil {
		return nil, fmt.Errorf("get generalized index: %w", err)
	}
	proof, err := info.Prove(gi)
	if err != nil {
		return nil, fmt.Errorf("prove gindex %d: %w", gi, err)
	}
	return proof, nil
}

// anchor is the resolved first path element.
// e.g.,
// 1) path = "validators[0].pubkey", anchor = "validators[0]"
// 2) path = "eth1_data.deposit_count", anchor = "eth1_data"
type anchor struct {
	info   *query.SszInfo
	gindex uint64
	leaf   []byte
	proof  [][]byte
}

// getBeaconStateProof proves the given path using hybrid-approach.
// - Leverage the native state's proof generation for anchor fields (= top-level fields e.g., "validators", "latest_block_header").
// - If needed, use generic proof collector for deeper fields, starting from the anchor (e.g., "validators[0].effective_balance").
func getBeaconStateProof(ctx context.Context, st state.BeaconState, info *query.SszInfo, path query.Path) (*sszquerypb.SSZQueryProof, error) {
	if len(path.Elements) == 0 {
		return nil, errors.New("cannot compute proof for empty path")
	}

	a, err := resolveAnchor(ctx, st, info, path.Elements[0])
	if err != nil {
		return nil, err
	}

	// If the query is only for the anchor field,
	// return the anchor proof directly without invoking the generic prover.
	if len(path.Elements) == 1 {
		return &sszquerypb.SSZQueryProof{
			Leaf:   a.leaf,
			Proofs: a.proof,
			Gindex: a.gindex,
		}, nil
	}

	// For deeper paths, use the generic prover rooted at the anchor.
	return deepenProof(info, path, a)
}

// resolveAnchor resolves the first path element into an anchor whose proof reaches the state root.
func resolveAnchor(ctx context.Context, st state.BeaconState, info *query.SszInfo, field query.PathElement) (*anchor, error) {
	name := field.Name

	beaconStateInfo, err := info.ContainerInfo()
	if err != nil {
		return nil, fmt.Errorf("could not get container info of BeaconState: %w", err)
	}

	fieldInfo, err := beaconStateInfo.FieldInfo(name)
	if err != nil {
		return nil, fmt.Errorf("could not get field info for anchor field %q: %w", name, err)
	}

	pos, ok := beaconStateInfo.FieldPosition(name)
	if !ok {
		return nil, fmt.Errorf("unknown field name: %s", name)
	}

	gindex, err := query.GetGeneralizedIndexFromPath(info, query.Path{Elements: []query.PathElement{field}})
	if err != nil {
		return nil, fmt.Errorf("could not compute gindex for anchor field %q: %w", name, err)
	}

	// Top-level field without index.
	if field.Index == nil {
		leaf, proof, err := st.ProofByFieldPosition(ctx, pos)
		if err != nil {
			return nil, fmt.Errorf("could not compute proof for anchor field %q: %w", name, err)
		}
		return &anchor{
			info:   fieldInfo,
			gindex: gindex,
			leaf:   leaf,
			proof:  proof,
		}, nil
	}

	// Note: From here on, the anchor field is expected
	// to be a List/Vector with index access (e.g., validators[0]).

	// elemInfo will be a new SszInfo for the element type.
	elemInfo, err := elementInfo(fieldInfo, field)
	if err != nil {
		return nil, err
	}

	leaf, proof, err := proveElement(ctx, st, info, field, pos, gindex)
	if err != nil {
		return nil, err
	}

	return &anchor{
		info:   elemInfo,
		gindex: gindex,
		leaf:   leaf,
		proof:  proof,
	}, nil
}

// elementInfo descends a List/Vector field's SszInfo to its element type, wiring the
// element value as the source (lists) for deeper proofs.
func elementInfo(fieldInfo *query.SszInfo, field query.PathElement) (*query.SszInfo, error) {
	name := field.Name

	switch fieldInfo.Type() {
	case query.List:
		li, err := fieldInfo.ListInfo()
		if err != nil {
			return nil, fmt.Errorf("could not get list info for field %q: %w", name, err)
		}

		elemInfo, err := li.Element()
		if err != nil {
			return nil, fmt.Errorf("could not get element info for list field %q: %w", name, err)
		}

		elementValue, err := li.ElementValue(int(*field.Index)) // lint:ignore uintcast -- BeaconState's list fields are not expected to exceed int64 max value.
		if err != nil {
			return nil, fmt.Errorf("could not get reflect.Value for list element %s[%d]: %w", name, *field.Index, err)
		}

		if sszObj, ok := elementValue.Interface().(query.SSZObject); ok {
			elemInfo.SetSource(sszObj)
		}

		return elemInfo, nil
	case query.Vector:
		vi, err := fieldInfo.VectorInfo()
		if err != nil {
			return nil, fmt.Errorf("could not get vector info for field %q: %w", name, err)
		}

		elemInfo, err := vi.Element()
		if err != nil {
			return nil, fmt.Errorf("could not get element info for vector field %q: %w", name, err)
		}

		return elemInfo, nil
	default:
		return nil, fmt.Errorf("field %q is not a List or Vector, cannot access by index", name)
	}
}

// proveElement proves field[index] to the state root, falling back to the generic prover
// for fields with no native field trie (e.g. slashings, historical_roots).
func proveElement(ctx context.Context, st state.BeaconState, info *query.SszInfo, field query.PathElement, pos int, gindex uint64) ([]byte, [][]byte, error) {
	index := *field.Index

	leaf, proof, err := st.ProofForFieldElement(ctx, pos, index)
	switch {
	case errors.Is(err, state.ErrFieldElementProofUnsupported):
		result, pErr := info.Prove(gindex)
		if pErr != nil {
			return nil, nil, fmt.Errorf("could not compute fallback proof for element %s[%d]: %w", field.Name, index, pErr)
		}
		return result.Leaf, result.Hashes, nil
	case err != nil:
		return nil, nil, fmt.Errorf("could not compute proof for element %s[%d]: %w", field.Name, index, err)
	default:
		return leaf, proof, nil
	}
}

// deepenProof extends the anchor proof to a deeper path via the generic prover.
func deepenProof(info *query.SszInfo, path query.Path, a *anchor) (*sszquerypb.SSZQueryProof, error) {
	targetGindex, err := query.GetGeneralizedIndexFromPath(info, path)
	if err != nil {
		return nil, fmt.Errorf("could not compute full gindex: %w", err)
	}

	relativeGindex, err := query.ComputeRelativeGindex(a.gindex, targetGindex)
	if err != nil {
		return nil, fmt.Errorf("could not compute relative gindex from the anchor: %w", err)
	}

	bottomProof, err := a.info.Prove(relativeGindex)
	if err != nil {
		return nil, fmt.Errorf("could not generate proof starting from the anchor: %w", err)
	}

	return &sszquerypb.SSZQueryProof{
		Leaf:   bottomProof.Leaf,
		Proofs: append(bottomProof.Hashes, a.proof...), // decreasing gindex order
		Gindex: targetGindex,
	}, nil
}
