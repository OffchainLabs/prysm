package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache/depositsnapshot"
	corehelpers "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filters"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/shared"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/prysm/v1alpha1/validator"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

const (
	broadcastValidationQueryParam               = "broadcast_validation"
	broadcastValidationConsensus                = "consensus"
	broadcastValidationConsensusAndEquivocation = "consensus_and_equivocation"
)

var (
	errNilBlock         = errors.New("nil block")
	errEquivocatedBlock = errors.New("block is equivocated")
	errMarshalSSZ       = errors.New("could not marshal block into SSZ")
)

// GetBlockV2 retrieves block details for given block ID.
func (s *Server) GetBlockV2(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetBlockV2")
	defer span.End()

	blockId := r.PathValue("block_id")
	if blockId == "" {
		httputil.HandleError(w, "block_id is required in URL params", http.StatusBadRequest)
		return
	}
	blk, err := s.Blocker.Block(ctx, []byte(blockId))
	if !shared.WriteBlockFetchError(w, blk, err) {
		return
	}

	// Deal with block unblinding.
	if blk.Version() >= version.Bellatrix && blk.IsBlinded() {
		blk, err = s.ExecutionReconstructor.ReconstructFullBlock(ctx, blk)
		if err != nil {
			httputil.HandleError(w, errors.Wrapf(err, "could not reconstruct full execution payload to create signed beacon block").Error(), http.StatusBadRequest)
			return
		}
	}

	if httputil.RespondWithSsz(r) {
		s.getBlockV2Ssz(w, blk)
	} else {
		s.getBlockV2Json(ctx, w, blk)
	}
}

// GetBlindedBlock retrieves blinded block for given block id.
func (s *Server) GetBlindedBlock(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetBlindedBlock")
	defer span.End()

	blockId := r.PathValue("block_id")
	if blockId == "" {
		httputil.HandleError(w, "block_id is required in URL params", http.StatusBadRequest)
		return
	}
	blk, err := s.Blocker.Block(ctx, []byte(blockId))
	if !shared.WriteBlockFetchError(w, blk, err) {
		return
	}

	// Convert to blinded block (if it's not already).
	if blk.Version() >= version.Bellatrix && !blk.IsBlinded() {
		blk, err = blk.ToBlinded()
		if err != nil {
			shared.WriteBlockFetchError(w, blk, errors.Wrapf(err, "could not convert block to blinded block"))
			return
		}
	}

	if httputil.RespondWithSsz(r) {
		s.getBlockV2Ssz(w, blk)
	} else {
		s.getBlockV2Json(ctx, w, blk)
	}
}

// getBlockV2Ssz returns the SSZ-serialized version of the beacon block for given block ID.
func (s *Server) getBlockV2Ssz(w http.ResponseWriter, blk interfaces.ReadOnlySignedBeaconBlock) {
	result, err := s.getBlockResponseBodySsz(blk)
	if err != nil {
		httputil.HandleError(w, "Could not get signed beacon block: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if result == nil {
		httputil.HandleError(w, fmt.Sprintf("Unknown block type %T", blk), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(blk.Version()))
	httputil.WriteSsz(w, result, "beacon_block.ssz")
}

func (*Server) getBlockResponseBodySsz(blk interfaces.ReadOnlySignedBeaconBlock) ([]byte, error) {
	err := blocks.BeaconBlockIsNil(blk)
	if err != nil {
		return nil, errNilBlock
	}
	pb, err := blk.Proto()
	if err != nil {
		return nil, err
	}
	marshaler, ok := pb.(ssz.Marshaler)
	if !ok {
		return nil, errMarshalSSZ
	}
	sszData, err := marshaler.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrapf(err, "could not marshal block into SSZ")
	}
	return sszData, nil
}

// getBlockV2Json returns the JSON-serialized version of the beacon block for given block ID.
func (s *Server) getBlockV2Json(ctx context.Context, w http.ResponseWriter, blk interfaces.ReadOnlySignedBeaconBlock) {
	result, err := s.getBlockResponseBodyJson(ctx, blk)
	if err != nil {
		httputil.HandleError(w, "Error processing request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if result == nil {
		httputil.HandleError(w, fmt.Sprintf("Unknown block type %T", blk), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, result.Version)
	httputil.WriteJson(w, result)
}

func (s *Server) getBlockResponseBodyJson(ctx context.Context, blk interfaces.ReadOnlySignedBeaconBlock) (*structs.GetBlockV2Response, error) {
	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		return nil, err
	}
	blkRoot, err := blk.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get block root")
	}
	finalized := s.FinalizationFetcher.IsFinalized(ctx, blkRoot)
	isOptimistic := false
	if blk.Version() >= version.Bellatrix {
		isOptimistic, err = s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, blkRoot)
		if err != nil {
			return nil, errors.Wrap(err, "could not check if block is optimistic")
		}
	}
	mj, err := structs.SignedBeaconBlockMessageJsoner(blk)
	if err != nil {
		return nil, err
	}
	jb, err := mj.MessageRawJson()
	if err != nil {
		return nil, err
	}
	return &structs.GetBlockV2Response{
		Finalized:           finalized,
		ExecutionOptimistic: isOptimistic,
		Version:             version.String(blk.Version()),
		Data: &structs.SignedBlock{
			Message:   jb,
			Signature: mj.SigString(),
		},
	}, nil
}

// GetBlockAttestations retrieves attestation included in requested block.
func (s *Server) GetBlockAttestations(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetBlockAttestations")
	defer span.End()

	blk, isOptimistic, root := s.blockData(ctx, w, r)
	if blk == nil {
		return
	}
	consensusAtts := blk.Block().Body().Attestations()
	atts := make([]*structs.Attestation, len(consensusAtts))
	for i, att := range consensusAtts {
		a, ok := att.(*eth.Attestation)
		if ok {
			atts[i] = structs.AttFromConsensus(a)
		} else {
			httputil.HandleError(w, fmt.Sprintf("unable to convert consensus attestations of type %T", att), http.StatusInternalServerError)
			return
		}
	}
	resp := &structs.GetBlockAttestationsResponse{
		Data:                atts,
		ExecutionOptimistic: isOptimistic,
		Finalized:           s.FinalizationFetcher.IsFinalized(ctx, root),
	}
	httputil.WriteJson(w, resp)
}

// GetBlockAttestationsV2 retrieves attestation included in requested block.
func (s *Server) GetBlockAttestationsV2(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetBlockAttestationsV2")
	defer span.End()

	blk, isOptimistic, root := s.blockData(ctx, w, r)
	if blk == nil {
		return
	}
	consensusAtts := blk.Block().Body().Attestations()

	v := blk.Block().Version()
	var attStructs []interface{}
	if v >= version.Electra {
		for _, att := range consensusAtts {
			a, ok := att.(*eth.AttestationElectra)
			if !ok {
				httputil.HandleError(w, fmt.Sprintf("unable to convert consensus attestations electra of type %T", att), http.StatusInternalServerError)
				return
			}
			attStruct := structs.AttElectraFromConsensus(a)
			attStructs = append(attStructs, attStruct)
		}
	} else {
		for _, att := range consensusAtts {
			a, ok := att.(*eth.Attestation)
			if !ok {
				httputil.HandleError(w, fmt.Sprintf("unable to convert consensus attestation of type %T", att), http.StatusInternalServerError)
				return
			}
			attStruct := structs.AttFromConsensus(a)
			attStructs = append(attStructs, attStruct)
		}
	}

	attBytes, err := json.Marshal(attStructs)
	if err != nil {
		httputil.HandleError(w, fmt.Sprintf("failed to marshal attestations: %v", err), http.StatusInternalServerError)
		return
	}
	resp := &structs.GetBlockAttestationsV2Response{
		Version:             version.String(v),
		ExecutionOptimistic: isOptimistic,
		Finalized:           s.FinalizationFetcher.IsFinalized(ctx, root),
		Data:                attBytes,
	}
	w.Header().Set(api.VersionHeader, version.String(v))
	httputil.WriteJson(w, resp)
}

func (s *Server) blockData(ctx context.Context, w http.ResponseWriter, r *http.Request) (interfaces.ReadOnlySignedBeaconBlock, bool, [32]byte) {
	blockId := r.PathValue("block_id")
	if blockId == "" {
		httputil.HandleError(w, "block_id is required in URL params", http.StatusBadRequest)
		return nil, false, [32]byte{}
	}
	blk, err := s.Blocker.Block(ctx, []byte(blockId))
	if !shared.WriteBlockFetchError(w, blk, err) {
		return nil, false, [32]byte{}
	}

	root, err := blk.Block().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not get block root: "+err.Error(), http.StatusInternalServerError)
		return nil, false, [32]byte{}
	}
	isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, root)
	if err != nil {
		httputil.HandleError(w, "Could not check if block is optimistic: "+err.Error(), http.StatusInternalServerError)
		return nil, false, [32]byte{}
	}
	return blk, isOptimistic, root
}

// PublishBlindedBlock instructs the beacon node to use the components of the `SignedBlindedBeaconBlock` to construct
// and publish a SignedBeaconBlock by swapping out the transactions_root for the corresponding full list of `transactions`.
// The beacon node should broadcast a newly constructed SignedBeaconBlock to the beacon network, to be included in the
// beacon chain. The beacon node is not required to validate the signed BeaconBlock, and a successful response (20X)
// only indicates that the broadcast has been successful. The beacon node is expected to integrate the new block into
// its state, and therefore validate the block internally, however blocks which fail the validation are still broadcast
// but a different status code is returned (202). Pre-Bellatrix, this endpoint will accept a SignedBeaconBlock. After
// Deneb, this additionally instructs the beacon node to broadcast all given signed blobs.
func (s *Server) PublishBlindedBlock(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishBlindedBlock")
	defer span.End()
	if shared.IsSyncing(r.Context(), w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}
	if httputil.IsRequestSsz(r) {
		s.publishBlindedBlockSSZ(ctx, w, r, false)
	} else {
		s.publishBlindedBlock(ctx, w, r, false)
	}
}

// PublishBlindedBlockV2 instructs the beacon node to use the components of the `SignedBlindedBeaconBlock` to construct and publish a
// `SignedBeaconBlock` by swapping out the `transactions_root` for the corresponding full list of `transactions`.
// The beacon node should broadcast a newly constructed `SignedBeaconBlock` to the beacon network,
// to be included in the beacon chain. The beacon node is not required to validate the signed
// `BeaconBlock`, and a successful response (20X) only indicates that the broadcast has been
// successful. The beacon node is expected to integrate the new block into its state, and
// therefore validate the block internally, however blocks which fail the validation are still
// broadcast but a different status code is returned (202). Pre-Bellatrix, this endpoint will accept
// a `SignedBeaconBlock`. After Deneb, this additionally instructs the beacon node to broadcast all given signed blobs.
// The broadcast behaviour may be adjusted via the `broadcast_validation`
// query parameter.
func (s *Server) PublishBlindedBlockV2(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishBlindedBlockV2")
	defer span.End()
	if shared.IsSyncing(r.Context(), w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}
	if httputil.IsRequestSsz(r) {
		s.publishBlindedBlockSSZ(ctx, w, r, true)
	} else {
		s.publishBlindedBlock(ctx, w, r, true)
	}
}

func (s *Server) publishBlindedBlockSSZ(ctx context.Context, w http.ResponseWriter, r *http.Request, versionRequired bool) { // nolint:gocognit
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.HandleError(w, "Could not read request body: "+err.Error(), http.StatusInternalServerError)
		return
	}
	versionHeader := r.Header.Get(api.VersionHeader)
	if versionRequired && versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
	}

	electraBlock := &eth.SignedBlindedBeaconBlockElectra{}
	if err = electraBlock.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_BlindedElectra{
				BlindedElectra: electraBlock,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Electra) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Electra), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	denebBlock := &eth.SignedBlindedBeaconBlockDeneb{}
	if err = denebBlock.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_BlindedDeneb{
				BlindedDeneb: denebBlock,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Deneb) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Deneb), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	capellaBlock := &eth.SignedBlindedBeaconBlockCapella{}
	if err = capellaBlock.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_BlindedCapella{
				BlindedCapella: capellaBlock,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Capella) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Capella), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	bellatrixBlock := &eth.SignedBlindedBeaconBlockBellatrix{}
	if err = bellatrixBlock.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_BlindedBellatrix{
				BlindedBellatrix: bellatrixBlock,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Bellatrix) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Bellatrix), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	altairBlock := &eth.SignedBeaconBlockAltair{}
	if err = altairBlock.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Altair{
				Altair: altairBlock,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Altair) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Altair), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	phase0Block := &eth.SignedBeaconBlock{}
	if err = phase0Block.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Phase0{
				Phase0: phase0Block,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Phase0) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Phase0), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	httputil.HandleError(w, "Body does not represent a valid block type", http.StatusBadRequest)
}

func (s *Server) publishBlindedBlock(ctx context.Context, w http.ResponseWriter, r *http.Request, versionRequired bool) { // nolint:gocognit
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.HandleError(w, "Could not read request body", http.StatusInternalServerError)
		return
	}
	versionHeader := r.Header.Get(api.VersionHeader)
	if versionRequired && versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
	}

	var consensusBlock *eth.GenericSignedBeaconBlock

	var electraBlock *structs.SignedBlindedBeaconBlockElectra
	if err = unmarshalStrict(body, &electraBlock); err == nil {
		consensusBlock, err = electraBlock.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Electra) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Electra), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	var denebBlock *structs.SignedBlindedBeaconBlockDeneb
	if err = unmarshalStrict(body, &denebBlock); err == nil {
		consensusBlock, err = denebBlock.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Deneb) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Deneb), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	var capellaBlock *structs.SignedBlindedBeaconBlockCapella
	if err = unmarshalStrict(body, &capellaBlock); err == nil {
		consensusBlock, err = capellaBlock.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Capella) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Capella), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	var bellatrixBlock *structs.SignedBlindedBeaconBlockBellatrix
	if err = unmarshalStrict(body, &bellatrixBlock); err == nil {
		consensusBlock, err = bellatrixBlock.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Bellatrix) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Bellatrix), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	var altairBlock *structs.SignedBeaconBlockAltair
	if err = unmarshalStrict(body, &altairBlock); err == nil {
		consensusBlock, err = altairBlock.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Altair) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Altair), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	var phase0Block *structs.SignedBeaconBlock
	if err = unmarshalStrict(body, &phase0Block); err == nil {
		consensusBlock, err = phase0Block.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Phase0) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Phase0), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	httputil.HandleError(w, "Body does not represent a valid block type", http.StatusBadRequest)
}

// PublishBlock instructs the beacon node to broadcast a newly signed beacon block to the beacon network,
// to be included in the beacon chain. A success response (20x) indicates that the block
// passed gossip validation and was successfully broadcast onto the network.
// The beacon node is also expected to integrate the block into state, but may broadcast it
// before doing so, so as to aid timely delivery of the block. Should the block fail full
// validation, a separate success response code (202) is used to indicate that the block was
// successfully broadcast but failed integration. After Deneb, this additionally instructs the
// beacon node to broadcast all given signed blobs.
func (s *Server) PublishBlock(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishBlock")
	defer span.End()
	if shared.IsSyncing(r.Context(), w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}
	if httputil.IsRequestSsz(r) {
		s.publishBlockSSZ(ctx, w, r, false)
	} else {
		s.publishBlock(ctx, w, r, false)
	}
}

// PublishBlockV2 instructs the beacon node to broadcast a newly signed beacon block to the beacon network,
// to be included in the beacon chain. A success response (20x) indicates that the block
// passed gossip validation and was successfully broadcast onto the network.
// The beacon node is also expected to integrate the block into the state, but may broadcast it
// before doing so, so as to aid timely delivery of the block. Should the block fail full
// validation, a separate success response code (202) is used to indicate that the block was
// successfully broadcast but failed integration. After Deneb, this additionally instructs the beacon node to
// broadcast all given signed blobs. The broadcast behaviour may be adjusted via the
// `broadcast_validation` query parameter.
func (s *Server) PublishBlockV2(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishBlockV2")
	defer span.End()
	if shared.IsSyncing(r.Context(), w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}
	if httputil.IsRequestSsz(r) {
		s.publishBlockSSZ(ctx, w, r, true)
	} else {
		s.publishBlock(ctx, w, r, true)
	}
}

func (s *Server) publishBlockSSZ(ctx context.Context, w http.ResponseWriter, r *http.Request, versionRequired bool) { // nolint:gocognit
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.HandleError(w, "Could not read request body", http.StatusInternalServerError)
		return
	}
	versionHeader := r.Header.Get(api.VersionHeader)
	if versionRequired && versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
		return
	}

	electraBlock := &eth.SignedBeaconBlockContentsElectra{}
	if err = electraBlock.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Electra{
				Electra: electraBlock,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			if errors.Is(err, errEquivocatedBlock) {
				b, err := blocks.NewSignedBeaconBlock(genericBlock)
				if err != nil {
					httputil.HandleError(w, err.Error(), http.StatusBadRequest)
					return
				}
				if err := s.broadcastSeenBlockSidecars(ctx, b, genericBlock.GetElectra().Blobs, genericBlock.GetElectra().KzgProofs); err != nil {
					log.WithError(err).Error("Failed to broadcast blob sidecars")
				}
			}
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Electra) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Electra), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	denebBlock := &eth.SignedBeaconBlockContentsDeneb{}
	if err = denebBlock.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Deneb{
				Deneb: denebBlock,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			if errors.Is(err, errEquivocatedBlock) {
				b, err := blocks.NewSignedBeaconBlock(genericBlock)
				if err != nil {
					httputil.HandleError(w, err.Error(), http.StatusBadRequest)
					return
				}
				if err := s.broadcastSeenBlockSidecars(ctx, b, genericBlock.GetDeneb().Blobs, genericBlock.GetDeneb().KzgProofs); err != nil {
					log.WithError(err).Error("Failed to broadcast blob sidecars")
				}
			}
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Deneb) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Deneb), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	capellaBlock := &eth.SignedBeaconBlockCapella{}
	if err = capellaBlock.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Capella{
				Capella: capellaBlock,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Capella) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Capella), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	bellatrixBlock := &eth.SignedBeaconBlockBellatrix{}
	if err = bellatrixBlock.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Bellatrix{
				Bellatrix: bellatrixBlock,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Bellatrix) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Bellatrix), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	altairBlock := &eth.SignedBeaconBlockAltair{}
	if err = altairBlock.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Altair{
				Altair: altairBlock,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Altair) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Altair), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	phase0Block := &eth.SignedBeaconBlock{}
	if err = phase0Block.UnmarshalSSZ(body); err == nil {
		genericBlock := &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Phase0{
				Phase0: phase0Block,
			},
		}
		if err = s.validateBroadcast(ctx, r, genericBlock); err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.proposeBlock(ctx, w, genericBlock)
		return
	}
	if versionHeader == version.String(version.Phase0) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Phase0), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	httputil.HandleError(w, "Body does not represent a valid block type", http.StatusBadRequest)
}

func (s *Server) publishBlock(ctx context.Context, w http.ResponseWriter, r *http.Request, versionRequired bool) { // nolint:gocognit
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.HandleError(w, "Could not read request body", http.StatusInternalServerError)
		return
	}
	versionHeader := r.Header.Get(api.VersionHeader)
	if versionRequired && versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
		return
	}

	var consensusBlock *eth.GenericSignedBeaconBlock

	var electraBlockContents *structs.SignedBeaconBlockContentsElectra
	if err = unmarshalStrict(body, &electraBlockContents); err == nil {
		consensusBlock, err = electraBlockContents.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				if errors.Is(err, errEquivocatedBlock) {
					b, err := blocks.NewSignedBeaconBlock(consensusBlock)
					if err != nil {
						httputil.HandleError(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := s.broadcastSeenBlockSidecars(ctx, b, consensusBlock.GetElectra().Blobs, consensusBlock.GetElectra().KzgProofs); err != nil {
						log.WithError(err).Error("Failed to broadcast blob sidecars")
					}
				}
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Electra) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Electra), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	var denebBlockContents *structs.SignedBeaconBlockContentsDeneb
	if err = unmarshalStrict(body, &denebBlockContents); err == nil {
		consensusBlock, err = denebBlockContents.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				if errors.Is(err, errEquivocatedBlock) {
					b, err := blocks.NewSignedBeaconBlock(consensusBlock)
					if err != nil {
						httputil.HandleError(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := s.broadcastSeenBlockSidecars(ctx, b, consensusBlock.GetDeneb().Blobs, consensusBlock.GetDeneb().KzgProofs); err != nil {
						log.WithError(err).Error("Failed to broadcast blob sidecars")
					}
				}
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Deneb) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Deneb), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	var capellaBlock *structs.SignedBeaconBlockCapella
	if err = unmarshalStrict(body, &capellaBlock); err == nil {
		consensusBlock, err = capellaBlock.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Capella) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Capella), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	var bellatrixBlock *structs.SignedBeaconBlockBellatrix
	if err = unmarshalStrict(body, &bellatrixBlock); err == nil {
		consensusBlock, err = bellatrixBlock.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Bellatrix) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Bellatrix), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	var altairBlock *structs.SignedBeaconBlockAltair
	if err = unmarshalStrict(body, &altairBlock); err == nil {
		consensusBlock, err = altairBlock.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Altair) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Altair), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	var phase0Block *structs.SignedBeaconBlock
	if err = unmarshalStrict(body, &phase0Block); err == nil {
		consensusBlock, err = phase0Block.ToGeneric()
		if err == nil {
			if err = s.validateBroadcast(ctx, r, consensusBlock); err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.proposeBlock(ctx, w, consensusBlock)
			return
		}
	}
	if versionHeader == version.String(version.Phase0) {
		httputil.HandleError(
			w,
			fmt.Sprintf("Could not decode request body into %s consensus block: %v", version.String(version.Phase0), err.Error()),
			http.StatusBadRequest,
		)
		return
	}

	httputil.HandleError(w, "Body does not represent a valid block type", http.StatusBadRequest)
}

func (s *Server) proposeBlock(ctx context.Context, w http.ResponseWriter, blk *eth.GenericSignedBeaconBlock) {
	_, err := s.V1Alpha1ValidatorServer.ProposeBeaconBlock(ctx, blk)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func unmarshalStrict(data []byte, v interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *Server) validateBroadcast(ctx context.Context, r *http.Request, blk *eth.GenericSignedBeaconBlock) error {
	switch r.URL.Query().Get(broadcastValidationQueryParam) {
	case broadcastValidationConsensus:
		b, err := blocks.NewSignedBeaconBlock(blk.Block)
		if err != nil {
			return errors.Wrapf(err, "could not create signed beacon block")
		}
		if err = s.validateConsensus(ctx, b); err != nil {
			return errors.Wrap(err, "consensus validation failed")
		}
	case broadcastValidationConsensusAndEquivocation:
		b, err := blocks.NewSignedBeaconBlock(blk.Block)
		if err != nil {
			return errors.Wrapf(err, "could not create signed beacon block")
		}
		if err = s.validateConsensus(r.Context(), b); err != nil {
			return errors.Wrap(err, "consensus validation failed")
		}
		if err = s.validateEquivocation(b.Block()); err != nil {
			return errors.Wrap(err, "equivocation validation failed")
		}
	default:
		return nil
	}
	return nil
}

func (s *Server) validateConsensus(ctx context.Context, blk interfaces.ReadOnlySignedBeaconBlock) error {
	parentBlockRoot := blk.Block().ParentRoot()
	parentBlock, err := s.Blocker.Block(ctx, parentBlockRoot[:])
	if err != nil {
		return errors.Wrap(err, "could not get parent block")
	}

	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		return errors.Wrap(err, "could not validate block")
	}

	parentStateRoot := parentBlock.Block().StateRoot()
	parentState, err := s.Stater.State(ctx, parentStateRoot[:])
	if err != nil {
		return errors.Wrap(err, "could not get parent state")
	}
	_, err = transition.ExecuteStateTransition(ctx, parentState, blk)
	if err != nil {
		return errors.Wrap(err, "could not execute state transition")
	}
	return nil
}

func (s *Server) validateEquivocation(blk interfaces.ReadOnlyBeaconBlock) error {
	slot, _ := s.ForkchoiceFetcher.HighestReceivedBlockSlotRoot()
	if slot == blk.Slot() {
		return errors.Wrapf(errEquivocatedBlock, "block for slot %d already exists in fork choice", blk.Slot())
	}
	return nil
}

// GetBlockRoot retrieves the root of a block.
func (s *Server) GetBlockRoot(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetBlockRoot")
	defer span.End()

	var err error
	var root []byte
	blockID := r.PathValue("block_id")
	if blockID == "" {
		httputil.HandleError(w, "block_id is required in URL params", http.StatusBadRequest)
		return
	}
	switch blockID {
	case "head":
		root, err = s.ChainInfoFetcher.HeadRoot(ctx)
		if err != nil {
			httputil.HandleError(w, "Could not retrieve head root: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if root == nil {
			httputil.HandleError(w, "No head root was found", http.StatusNotFound)
			return
		}
	case "finalized":
		finalized := s.ChainInfoFetcher.FinalizedCheckpt()
		root = finalized.Root
	case "genesis":
		blk, err := s.BeaconDB.GenesisBlock(ctx)
		if err != nil {
			httputil.HandleError(w, "Could not retrieve genesis block: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := blocks.BeaconBlockIsNil(blk); err != nil {
			httputil.HandleError(w, "Could not find genesis block: "+err.Error(), http.StatusNotFound)
			return
		}
		blkRoot, err := blk.Block().HashTreeRoot()
		if err != nil {
			httputil.HandleError(w, "Could not hash genesis block: "+err.Error(), http.StatusInternalServerError)
			return
		}
		root = blkRoot[:]
	default:
		isHex := strings.HasPrefix(blockID, "0x")
		if isHex {
			blockIDBytes, err := hexutil.Decode(blockID)
			if err != nil {
				httputil.HandleError(w, "Could not decode block ID into bytes: "+err.Error(), http.StatusBadRequest)
				return
			}
			if len(blockIDBytes) != fieldparams.RootLength {
				httputil.HandleError(w, fmt.Sprintf("Block ID has length %d instead of %d", len(blockIDBytes), fieldparams.RootLength), http.StatusBadRequest)
				return
			}
			blockID32 := bytesutil.ToBytes32(blockIDBytes)
			blk, err := s.BeaconDB.Block(ctx, blockID32)
			if err != nil {
				httputil.HandleError(w, fmt.Sprintf("Could not retrieve block for block root %#x: %v", blockID, err), http.StatusInternalServerError)
				return
			}
			if err := blocks.BeaconBlockIsNil(blk); err != nil {
				httputil.HandleError(w, "Could not find block: "+err.Error(), http.StatusNotFound)
				return
			}
			root = blockIDBytes
		} else {
			slot, err := strconv.ParseUint(blockID, 10, 64)
			if err != nil {
				httputil.HandleError(w, "Could not parse block ID: "+err.Error(), http.StatusBadRequest)
				return
			}
			hasRoots, roots, err := s.BeaconDB.BlockRootsBySlot(ctx, primitives.Slot(slot))
			if err != nil {
				httputil.HandleError(w, fmt.Sprintf("Could not retrieve blocks for slot %d: %v", slot, err), http.StatusInternalServerError)
				return
			}

			if !hasRoots {
				httputil.HandleError(w, "Could not find any blocks with given slot", http.StatusNotFound)
				return
			}
			root = roots[0][:]
			if len(roots) == 1 {
				break
			}
			for _, blockRoot := range roots {
				canonical, err := s.ChainInfoFetcher.IsCanonical(ctx, blockRoot)
				if err != nil {
					httputil.HandleError(w, "Could not determine if block root is canonical: "+err.Error(), http.StatusInternalServerError)
					return
				}
				if canonical {
					root = blockRoot[:]
					break
				}
			}
		}
	}

	b32Root := bytesutil.ToBytes32(root)
	isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, b32Root)
	if err != nil {
		httputil.HandleError(w, "Could not check if block is optimistic: "+err.Error(), http.StatusInternalServerError)
		return
	}
	response := &structs.BlockRootResponse{
		Data: &structs.BlockRoot{
			Root: hexutil.Encode(root),
		},
		ExecutionOptimistic: isOptimistic,
		Finalized:           s.FinalizationFetcher.IsFinalized(ctx, b32Root),
	}
	httputil.WriteJson(w, response)
}

// GetStateFork returns Fork object for state with given 'stateId'.
func (s *Server) GetStateFork(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetStateFork")
	defer span.End()

	stateId := r.PathValue("state_id")
	if stateId == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}
	st, err := s.Stater.State(ctx, []byte(stateId))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}
	fork := st.Fork()
	isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		httputil.HandleError(w, "Could not check optimistic status"+err.Error(), http.StatusInternalServerError)
		return
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, errors.Wrap(err, "Could not calculate root of latest block header: ").Error(), http.StatusInternalServerError)
		return
	}
	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)
	response := &structs.GetStateForkResponse{
		Data: &structs.Fork{
			PreviousVersion: hexutil.Encode(fork.PreviousVersion),
			CurrentVersion:  hexutil.Encode(fork.CurrentVersion),
			Epoch:           fmt.Sprintf("%d", fork.Epoch),
		},
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}
	httputil.WriteJson(w, response)
}

// GetCommittees retrieves the committees for the given state at the given epoch.
// If the requested slot and index are defined, only those committees are returned.
func (s *Server) GetCommittees(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetCommittees")
	defer span.End()

	stateId := r.PathValue("state_id")
	if stateId == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}

	rawEpoch, e, ok := shared.UintFromQuery(w, r, "epoch", false)
	if !ok {
		return
	}
	rawIndex, i, ok := shared.UintFromQuery(w, r, "index", false)
	if !ok {
		return
	}
	rawSlot, sl, ok := shared.UintFromQuery(w, r, "slot", false)
	if !ok {
		return
	}

	st, err := s.Stater.State(ctx, []byte(stateId))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	epoch := slots.ToEpoch(st.Slot())
	if rawEpoch != "" {
		epoch = primitives.Epoch(e)
	}
	activeCount, err := corehelpers.ActiveValidatorCount(ctx, st, epoch)
	if err != nil {
		httputil.HandleError(w, "Could not get active validator count: "+err.Error(), http.StatusInternalServerError)
		return
	}

	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		httputil.HandleError(w, "Could not get epoch start slot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	endSlot, err := slots.EpochEnd(epoch)
	if err != nil {
		httputil.HandleError(w, "Could not get epoch end slot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	committeesPerSlot := corehelpers.SlotCommitteeCount(activeCount)
	committees := make([]*structs.Committee, 0)
	for slot := startSlot; slot <= endSlot; slot++ {
		if rawSlot != "" && slot != primitives.Slot(sl) {
			continue
		}
		for index := primitives.CommitteeIndex(0); index < primitives.CommitteeIndex(committeesPerSlot); index++ {
			if rawIndex != "" && index != primitives.CommitteeIndex(i) {
				continue
			}
			committee, err := corehelpers.BeaconCommitteeFromState(ctx, st, slot, index)
			if err != nil {
				httputil.HandleError(w, "Could not get committee: "+err.Error(), http.StatusInternalServerError)
				return
			}
			var validators []string
			for _, v := range committee {
				validators = append(validators, strconv.FormatUint(uint64(v), 10))
			}
			committeeContainer := &structs.Committee{
				Index:      strconv.FormatUint(uint64(index), 10),
				Slot:       strconv.FormatUint(uint64(slot), 10),
				Validators: validators,
			}
			committees = append(committees, committeeContainer)
		}
	}

	isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		httputil.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)
	httputil.WriteJson(w, &structs.GetCommitteesResponse{Data: committees, ExecutionOptimistic: isOptimistic, Finalized: isFinalized})
}

// GetBlockHeaders retrieves block headers matching given query. By default it will fetch current head slot blocks.
func (s *Server) GetBlockHeaders(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetBlockHeaders")
	defer span.End()

	rawSlot, slot, ok := shared.UintFromQuery(w, r, "slot", false)
	if !ok {
		return
	}
	rawParentRoot, parentRoot, ok := shared.HexFromQuery(w, r, "parent_root", fieldparams.RootLength, false)
	if !ok {
		return
	}

	var err error
	var blks []interfaces.ReadOnlySignedBeaconBlock
	var blkRoots [][32]byte

	if rawParentRoot != "" {
		blks, blkRoots, err = s.BeaconDB.Blocks(ctx, filters.NewFilter().SetParentRoot(parentRoot))
		if err != nil {
			httputil.HandleError(w, errors.Wrapf(err, "Could not retrieve blocks for parent root %s", parentRoot).Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if rawSlot == "" {
			slot = uint64(s.ChainInfoFetcher.HeadSlot())
		}
		blks, err = s.BeaconDB.BlocksBySlot(ctx, primitives.Slot(slot))
		if err != nil {
			httputil.HandleError(w, errors.Wrapf(err, "Could not retrieve blocks for slot %d", slot).Error(), http.StatusInternalServerError)
			return
		}
		_, blkRoots, err = s.BeaconDB.BlockRootsBySlot(ctx, primitives.Slot(slot))
		if err != nil {
			httputil.HandleError(w, errors.Wrapf(err, "Could not retrieve blocks for slot %d", slot).Error(), http.StatusInternalServerError)
			return
		}
	}

	if len(blks) == 0 {
		httputil.HandleError(w, "No blocks found", http.StatusNotFound)
		return
	}

	isOptimistic := false
	isFinalized := true
	blkHdrs := make([]*structs.SignedBeaconBlockHeaderContainer, len(blks))
	for i, bl := range blks {
		v1alpha1Header, err := bl.Header()
		if err != nil {
			httputil.HandleError(w, errors.Wrapf(err, "Could not get block header from block").Error(), http.StatusInternalServerError)
			return
		}
		headerRoot, err := v1alpha1Header.Header.HashTreeRoot()
		if err != nil {
			httputil.HandleError(w, errors.Wrapf(err, "Could not hash block header").Error(), http.StatusInternalServerError)
			return
		}
		canonical, err := s.ChainInfoFetcher.IsCanonical(ctx, blkRoots[i])
		if err != nil {
			httputil.HandleError(w, errors.Wrapf(err, "Could not determine if block root is canonical").Error(), http.StatusInternalServerError)
			return
		}
		if !isOptimistic {
			isOptimistic, err = s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, blkRoots[i])
			if err != nil {
				httputil.HandleError(w, errors.Wrapf(err, "Could not check if block is optimistic").Error(), http.StatusInternalServerError)
				return
			}
		}
		if isFinalized {
			isFinalized = s.FinalizationFetcher.IsFinalized(ctx, blkRoots[i])
		}
		blkHdrs[i] = &structs.SignedBeaconBlockHeaderContainer{
			Header: &structs.SignedBeaconBlockHeader{
				Message:   structs.BeaconBlockHeaderFromConsensus(v1alpha1Header.Header),
				Signature: hexutil.Encode(v1alpha1Header.Signature),
			},
			Root:      hexutil.Encode(headerRoot[:]),
			Canonical: canonical,
		}
	}

	response := &structs.GetBlockHeadersResponse{
		Data:                blkHdrs,
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}
	httputil.WriteJson(w, response)
}

// GetBlockHeader retrieves block header for given block id.
func (s *Server) GetBlockHeader(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetBlockHeader")
	defer span.End()

	blockID := r.PathValue("block_id")
	if blockID == "" {
		httputil.HandleError(w, "block_id is required in URL params", http.StatusBadRequest)
		return
	}

	blk, err := s.Blocker.Block(ctx, []byte(blockID))
	ok := shared.WriteBlockFetchError(w, blk, err)
	if !ok {
		return
	}
	blockHeader, err := blk.Header()
	if err != nil {
		httputil.HandleError(w, "Could not get block header: %s"+err.Error(), http.StatusInternalServerError)
		return
	}
	headerRoot, err := blockHeader.Header.HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not hash block header: %s"+err.Error(), http.StatusInternalServerError)
		return
	}
	blkRoot, err := blk.Block().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not hash block: %s"+err.Error(), http.StatusInternalServerError)
		return
	}
	canonical, err := s.ChainInfoFetcher.IsCanonical(ctx, blkRoot)
	if err != nil {
		httputil.HandleError(w, "Could not determine if block root is canonical: %s"+err.Error(), http.StatusInternalServerError)
		return
	}
	isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, blkRoot)
	if err != nil {
		httputil.HandleError(w, "Could not check if block is optimistic: %s"+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := &structs.GetBlockHeaderResponse{
		Data: &structs.SignedBeaconBlockHeaderContainer{
			Root:      hexutil.Encode(headerRoot[:]),
			Canonical: canonical,
			Header: &structs.SignedBeaconBlockHeader{
				Message:   structs.BeaconBlockHeaderFromConsensus(blockHeader.Header),
				Signature: hexutil.Encode(blockHeader.Signature),
			},
		},
		ExecutionOptimistic: isOptimistic,
		Finalized:           s.FinalizationFetcher.IsFinalized(ctx, blkRoot),
	}
	httputil.WriteJson(w, resp)
}

// GetFinalityCheckpoints returns finality checkpoints for state with given 'stateId'. In case finality is
// not yet achieved, checkpoint should return epoch 0 and ZERO_HASH as root.
func (s *Server) GetFinalityCheckpoints(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetFinalityCheckpoints")
	defer span.End()

	stateId := r.PathValue("state_id")
	if stateId == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}

	st, err := s.Stater.State(ctx, []byte(stateId))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}
	isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		httputil.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	pj := st.PreviousJustifiedCheckpoint()
	cj := st.CurrentJustifiedCheckpoint()
	f := st.FinalizedCheckpoint()
	resp := &structs.GetFinalityCheckpointsResponse{
		Data: &structs.FinalityCheckpoints{
			PreviousJustified: &structs.Checkpoint{
				Epoch: strconv.FormatUint(uint64(pj.Epoch), 10),
				Root:  hexutil.Encode(pj.Root),
			},
			CurrentJustified: &structs.Checkpoint{
				Epoch: strconv.FormatUint(uint64(cj.Epoch), 10),
				Root:  hexutil.Encode(cj.Root),
			},
			Finalized: &structs.Checkpoint{
				Epoch: strconv.FormatUint(uint64(f.Epoch), 10),
				Root:  hexutil.Encode(f.Root),
			},
		},
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}
	httputil.WriteJson(w, resp)
}

// GetGenesis retrieves details of the chain's genesis which can be used to identify chain.
func (s *Server) GetGenesis(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "beacon.GetGenesis")
	defer span.End()

	genesisTime := s.GenesisTimeFetcher.GenesisTime()
	if genesisTime.IsZero() {
		httputil.HandleError(w, "Chain genesis info is not yet known", http.StatusNotFound)
		return
	}
	validatorsRoot := s.ChainInfoFetcher.GenesisValidatorsRoot()
	if bytes.Equal(validatorsRoot[:], params.BeaconConfig().ZeroHash[:]) {
		httputil.HandleError(w, "Chain genesis info is not yet known", http.StatusNotFound)
		return
	}
	forkVersion := params.BeaconConfig().GenesisForkVersion

	resp := &structs.GetGenesisResponse{
		Data: &structs.Genesis{
			GenesisTime:           strconv.FormatUint(uint64(genesisTime.Unix()), 10),
			GenesisValidatorsRoot: hexutil.Encode(validatorsRoot[:]),
			GenesisForkVersion:    hexutil.Encode(forkVersion),
		},
	}
	httputil.WriteJson(w, resp)
}

// GetDepositSnapshot retrieves the EIP-4881 Deposit Tree Snapshot. Either a JSON or,
// if the Accept header was added, bytes serialized by SSZ will be returned.
func (s *Server) GetDepositSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetDepositSnapshot")
	defer span.End()

	eth1data, err := s.BeaconDB.ExecutionChainData(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not retrieve execution chain data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if eth1data == nil {
		httputil.HandleError(w, "Could not retrieve execution chain data: empty Eth1Data", http.StatusInternalServerError)
		return
	}
	snapshot := eth1data.DepositSnapshot
	if snapshot == nil || len(snapshot.Finalized) == 0 {
		httputil.HandleError(w, "No finalized snapshot available", http.StatusNotFound)
		return
	}
	if len(snapshot.Finalized) > depositsnapshot.DepositContractDepth {
		httputil.HandleError(w, "Retrieved invalid deposit snapshot", http.StatusInternalServerError)
		return
	}
	if httputil.RespondWithSsz(r) {
		sszData, err := snapshot.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "Could not marshal deposit snapshot into SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszData, "deposit_snapshot.ssz")
		return
	}
	httputil.WriteJson(
		w,
		&structs.GetDepositSnapshotResponse{
			Data: structs.DepositSnapshotFromConsensus(snapshot),
		},
	)
}

// Broadcast blob sidecars even if the block of the same slot has been imported.
// To ensure safety, we will only broadcast blob sidecars if the header references the same block that was previously seen.
// Otherwise, a proposer could get slashed through a different blob sidecar header reference.
func (s *Server) broadcastSeenBlockSidecars(
	ctx context.Context,
	b interfaces.SignedBeaconBlock,
	blobs [][]byte,
	kzgProofs [][]byte) error {
	scs, err := validator.BuildBlobSidecars(b, blobs, kzgProofs)
	if err != nil {
		return err
	}
	for _, sc := range scs {
		r, err := sc.SignedBlockHeader.Header.HashTreeRoot()
		if err != nil {
			log.WithError(err).Error("Failed to hash block header for blob sidecar")
			continue
		}
		if !s.FinalizationFetcher.InForkchoice(r) {
			log.WithField("root", fmt.Sprintf("%#x", r)).Debug("Block header not in forkchoice, skipping blob sidecar broadcast")
			continue
		}
		if err := s.Broadcaster.BroadcastBlob(ctx, sc.Index, sc); err != nil {
			log.WithError(err).Error("Failed to broadcast blob sidecar for index ", sc.Index)
		}
		log.WithFields(logrus.Fields{
			"index":         sc.Index,
			"slot":          sc.SignedBlockHeader.Header.Slot,
			"kzgCommitment": fmt.Sprintf("%#x", sc.KzgCommitment),
		}).Info("Broadcasted blob sidecar for already seen block")
	}
	return nil
}
