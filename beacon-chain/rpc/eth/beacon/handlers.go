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
	"time"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache/depositsnapshot"
	corehelpers "github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filters"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/prysm/v1alpha1/validator"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
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

type blockDecoder func([]byte) (*eth.GenericSignedBeaconBlock, error)

func decodingError(v string, err error) error {
	return fmt.Errorf("could not decode request body into %s consensus block: %w", v, err)
}

type signedBlockContentPeeker struct {
	Block json.RawMessage `json:"signed_block"`
}
type slotPeeker struct {
	Block struct {
		Slot primitives.Slot `json:"slot,string"`
	} `json:"message"`
}

func versionHeaderFromRequest(body []byte) (string, error) {
	// check is required for post deneb fork blocks contents
	p := &signedBlockContentPeeker{}
	if err := json.Unmarshal(body, p); err != nil {
		return "", errors.Wrap(err, "unable to peek slot from block contents")
	}
	data := body
	if len(p.Block) > 0 {
		data = p.Block
	}
	sp := &slotPeeker{}
	if err := json.Unmarshal(data, sp); err != nil {
		return "", errors.Wrap(err, "unable to peek slot from block")
	}
	ce := slots.ToEpoch(sp.Block.Slot)
	if ce >= params.BeaconConfig().FuluForkEpoch {
		return version.String(version.Fulu), nil
	} else if ce >= params.BeaconConfig().ElectraForkEpoch {
		return version.String(version.Electra), nil
	} else if ce >= params.BeaconConfig().DenebForkEpoch {
		return version.String(version.Deneb), nil
	} else if ce >= params.BeaconConfig().CapellaForkEpoch {
		return version.String(version.Capella), nil
	} else if ce >= params.BeaconConfig().BellatrixForkEpoch {
		return version.String(version.Bellatrix), nil
	} else if ce >= params.BeaconConfig().AltairForkEpoch {
		return version.String(version.Altair), nil
	} else {
		return version.String(version.Phase0), nil
	}
}

// validateVersionHeader checks if the version header is required and retrieves it
// from the request. If the version header is not provided and not required, it attempts
// to derive it from the request body.
func validateVersionHeader(r *http.Request, body []byte, versionRequired bool) (string, error) {
	versionHeader := r.Header.Get(api.VersionHeader)
	if versionRequired && versionHeader == "" {
		return "", fmt.Errorf("%s header is required", api.VersionHeader)
	}

	if !versionRequired && versionHeader == "" {
		var err error
		versionHeader, err = versionHeaderFromRequest(body)
		if err != nil {
			return "", errors.Wrap(err, "could not decode request body for version header")
		}
	}

	return versionHeader, nil
}

func readRequestBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(r.Body)
}

// GenericConverter is an example interface that your block structs could implement.
type GenericConverter interface {
	ToGeneric() (*eth.GenericSignedBeaconBlock, error)
}

// decodeGenericJSON uses generics to unmarshal JSON into a type T that also
// provides a ToGeneric() method to produce a *eth.GenericSignedBeaconBlock.
func decodeGenericJSON[T GenericConverter](body []byte, forkVersion string) (*eth.GenericSignedBeaconBlock, error) {
	// Create a pointer to the zero value of T.
	blockPtr := new(T)

	// Unmarshal JSON into blockPtr.
	if err := unmarshalStrict(body, blockPtr); err != nil {
		return nil, decodingError(forkVersion, err)
	}

	// Call the ToGeneric method on the underlying value.
	consensusBlock, err := (*blockPtr).ToGeneric()
	if err != nil {
		return nil, decodingError(forkVersion, err)
	}

	return consensusBlock, nil
}

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
	httputil.WriteSsz(w, result)
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

// Deprecated: use GetBlockAttestationsV2 instead
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
	attStructs := make([]interface{}, len(consensusAtts))
	if v >= version.Electra {
		for index, att := range consensusAtts {
			a, ok := att.(*eth.AttestationElectra)
			if !ok {
				httputil.HandleError(w, fmt.Sprintf("unable to convert consensus attestations electra of type %T", att), http.StatusInternalServerError)
				return
			}
			attStruct := structs.AttElectraFromConsensus(a)
			attStructs[index] = attStruct
		}
	} else {
		for index, att := range consensusAtts {
			a, ok := att.(*eth.Attestation)
			if !ok {
				httputil.HandleError(w, fmt.Sprintf("unable to convert consensus attestation of type %T", att), http.StatusInternalServerError)
				return
			}
			attStruct := structs.AttFromConsensus(a)
			attStructs[index] = attStruct
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

// Deprecated: use PublishBlindedBlockV2 instead
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

// publishBlindedBlockSSZ reads SSZ-encoded data and publishes a blinded block.
func (s *Server) publishBlindedBlockSSZ(ctx context.Context, w http.ResponseWriter, r *http.Request, versionRequired bool) {
	body, err := readRequestBody(r)
	if err != nil {
		httputil.HandleError(w, "Could not read request body: "+err.Error(), http.StatusInternalServerError)
		return
	}

	versionHeader, err := validateVersionHeader(r, body, versionRequired)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	genericBlock, err := decodeBlindedBlockSSZ(versionHeader, body)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.validateBroadcast(ctx, r, genericBlock); err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.proposeBlock(ctx, w, genericBlock)
}

// decodeBlindedBlockSSZ dispatches to the correct SSZ decoder based on versionHeader.
func decodeBlindedBlockSSZ(versionHeader string, body []byte) (*eth.GenericSignedBeaconBlock, error) {
	if decoder, exists := blindedSSZDecoders[versionHeader]; exists {
		return decoder(body)
	}
	return nil, fmt.Errorf("body does not represent a valid blinded block type")
}

var blindedSSZDecoders = map[string]blockDecoder{
	version.String(version.Fulu):      decodeBlindedFuluSSZ,
	version.String(version.Electra):   decodeBlindedElectraSSZ,
	version.String(version.Deneb):     decodeBlindedDenebSSZ,
	version.String(version.Capella):   decodeBlindedCapellaSSZ,
	version.String(version.Bellatrix): decodeBlindedBellatrixSSZ,
	version.String(version.Altair):    decodeAltairSSZ,
	version.String(version.Phase0):    decodePhase0SSZ,
}

func decodeBlindedFuluSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	fuluBlock := &eth.SignedBlindedBeaconBlockFulu{}
	if err := fuluBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(version.String(version.Fulu), err)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_BlindedFulu{
			BlindedFulu: fuluBlock,
		},
	}, nil
}

func decodeBlindedElectraSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	electraBlock := &eth.SignedBlindedBeaconBlockElectra{}
	if err := electraBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(version.String(version.Electra), err)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_BlindedElectra{
			BlindedElectra: electraBlock,
		},
	}, nil
}

func decodeBlindedDenebSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	denebBlock := &eth.SignedBlindedBeaconBlockDeneb{}
	if err := denebBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(version.String(version.Deneb), err)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_BlindedDeneb{
			BlindedDeneb: denebBlock,
		},
	}, nil
}

func decodeBlindedCapellaSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	capellaBlock := &eth.SignedBlindedBeaconBlockCapella{}
	if err := capellaBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(version.String(version.Capella), err)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_BlindedCapella{
			BlindedCapella: capellaBlock,
		},
	}, nil
}

func decodeBlindedBellatrixSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	bellatrixBlock := &eth.SignedBlindedBeaconBlockBellatrix{}
	if err := bellatrixBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(version.String(version.Bellatrix), err)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_BlindedBellatrix{
			BlindedBellatrix: bellatrixBlock,
		},
	}, nil
}

// publishBlindedBlock reads JSON-encoded data and publishes a blinded block.
func (s *Server) publishBlindedBlock(ctx context.Context, w http.ResponseWriter, r *http.Request, versionRequired bool) {
	body, err := readRequestBody(r)
	if err != nil {
		httputil.HandleError(w, "Could not read request body", http.StatusInternalServerError)
		return
	}

	versionHeader, err := validateVersionHeader(r, body, versionRequired)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	genericBlock, err := decodeBlindedBlockJSON(versionHeader, body)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.validateBroadcast(ctx, r, genericBlock); err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.proposeBlock(ctx, w, genericBlock)
}

// decodeBlindedBlockJSON dispatches to the correct JSON decoder based on versionHeader.
func decodeBlindedBlockJSON(versionHeader string, body []byte) (*eth.GenericSignedBeaconBlock, error) {
	if decoder, exists := blindedJSONDecoders[versionHeader]; exists {
		return decoder(body)
	}
	return nil, fmt.Errorf("body does not represent a valid blinded block type")
}

var blindedJSONDecoders = map[string]blockDecoder{
	version.String(version.Fulu):      decodeBlindedFuluJSON,
	version.String(version.Electra):   decodeBlindedElectraJSON,
	version.String(version.Deneb):     decodeBlindedDenebJSON,
	version.String(version.Capella):   decodeBlindedCapellaJSON,
	version.String(version.Bellatrix): decodeBlindedBellatrixJSON,
	version.String(version.Altair):    decodeAltairJSON,
	version.String(version.Phase0):    decodePhase0JSON,
}

func decodeBlindedFuluJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBlindedBeaconBlockFulu](
		body,
		version.String(version.Fulu),
	)
}

func decodeBlindedElectraJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBlindedBeaconBlockElectra](
		body,
		version.String(version.Electra),
	)
}

func decodeBlindedDenebJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBlindedBeaconBlockDeneb](
		body,
		version.String(version.Deneb),
	)
}

func decodeBlindedCapellaJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBlindedBeaconBlockCapella](
		body,
		version.String(version.Capella),
	)
}

func decodeBlindedBellatrixJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBlindedBeaconBlockBellatrix](
		body,
		version.String(version.Bellatrix),
	)
}

// Deprecated: use PublishBlockV2 instead
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
	start := time.Now()
	defer func() {
		duration := time.Since(start).Milliseconds()
		publishBlockV2Duration.Observe(float64(duration))
	}()

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

// publishBlockSSZ handles publishing an SSZ-encoded block to the beacon node.
func (s *Server) publishBlockSSZ(ctx context.Context, w http.ResponseWriter, r *http.Request, versionRequired bool) {
	body, err := readRequestBody(r)
	if err != nil {
		httputil.HandleError(w, "Could not read request body", http.StatusInternalServerError)
		return
	}

	versionHeader, err := validateVersionHeader(r, body, versionRequired)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Decode SSZ into a generic block.
	genericBlock, err := decodeSSZToGenericBlock(versionHeader, body)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate and optionally broadcast sidecars on equivocation.
	if err := s.validateBroadcast(ctx, r, genericBlock); err != nil {
		if errors.Is(err, errEquivocatedBlock) {
			b, err := blocks.NewSignedBeaconBlock(genericBlock.Block)
			if err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err = broadcastSidecarsIfSupported(ctx, s, b, genericBlock, versionHeader); err != nil {
				log.WithError(err).Error("Failed to broadcast blob sidecars")
			}
		}
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.proposeBlock(ctx, w, genericBlock)
}

var sszDecoders = map[string]blockDecoder{
	version.String(version.Fulu):      decodeFuluSSZ,
	version.String(version.Electra):   decodeElectraSSZ,
	version.String(version.Deneb):     decodeDenebSSZ,
	version.String(version.Capella):   decodeCapellaSSZ,
	version.String(version.Bellatrix): decodeBellatrixSSZ,
	version.String(version.Altair):    decodeAltairSSZ,
	version.String(version.Phase0):    decodePhase0SSZ,
}

// decodeSSZToGenericBlock uses a lookup table to map a version string to the proper decoder.
func decodeSSZToGenericBlock(versionHeader string, body []byte) (*eth.GenericSignedBeaconBlock, error) {
	if decoder, found := sszDecoders[versionHeader]; found {
		return decoder(body)
	}
	return nil, errors.New("body does not represent a valid block type")
}

func decodeFuluSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	fuluBlock := &eth.SignedBeaconBlockContentsFulu{}
	if err := fuluBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(
			version.String(version.Fulu), err,
		)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_Fulu{Fulu: fuluBlock},
	}, nil
}

func decodeElectraSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	electraBlock := &eth.SignedBeaconBlockContentsElectra{}
	if err := electraBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(
			version.String(version.Electra), err,
		)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_Electra{Electra: electraBlock},
	}, nil
}

func decodeDenebSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	denebBlock := &eth.SignedBeaconBlockContentsDeneb{}
	if err := denebBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(
			version.String(version.Deneb),
			err,
		)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_Deneb{
			Deneb: denebBlock,
		},
	}, nil
}

func decodeCapellaSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	capellaBlock := &eth.SignedBeaconBlockCapella{}
	if err := capellaBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(
			version.String(version.Capella),
			err,
		)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_Capella{
			Capella: capellaBlock,
		},
	}, nil
}

func decodeBellatrixSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	bellatrixBlock := &eth.SignedBeaconBlockBellatrix{}
	if err := bellatrixBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(
			version.String(version.Bellatrix),
			err,
		)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_Bellatrix{
			Bellatrix: bellatrixBlock,
		},
	}, nil
}

func decodeAltairSSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	altairBlock := &eth.SignedBeaconBlockAltair{}
	if err := altairBlock.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(
			version.String(version.Altair),
			err,
		)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_Altair{
			Altair: altairBlock,
		},
	}, nil
}

func decodePhase0SSZ(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	phase0Block := &eth.SignedBeaconBlock{}
	if err := phase0Block.UnmarshalSSZ(body); err != nil {
		return nil, decodingError(
			version.String(version.Phase0), err,
		)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_Phase0{Phase0: phase0Block},
	}, nil
}

// publishBlock handles publishing a JSON-encoded block to the beacon node.
func (s *Server) publishBlock(ctx context.Context, w http.ResponseWriter, r *http.Request, versionRequired bool) {
	body, err := readRequestBody(r)
	if err != nil {
		httputil.HandleError(w, "Could not read request body", http.StatusInternalServerError)
		return
	}

	versionHeader, err := validateVersionHeader(r, body, versionRequired)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Decode JSON into a generic block.
	genericBlock, decodeErr := decodeJSONToGenericBlock(versionHeader, body)
	if decodeErr != nil {
		httputil.HandleError(w, decodeErr.Error(), http.StatusBadRequest)
		return
	}

	// Validate and optionally broadcast sidecars on equivocation.
	if err := s.validateBroadcast(ctx, r, genericBlock); err != nil {
		if errors.Is(err, errEquivocatedBlock) {
			b, err := blocks.NewSignedBeaconBlock(genericBlock.Block)
			if err != nil {
				httputil.HandleError(w, err.Error(), http.StatusBadRequest)
				return
			}

			if err := broadcastSidecarsIfSupported(ctx, s, b, genericBlock, versionHeader); err != nil {
				log.WithError(err).Error("Failed to broadcast blob sidecars")
			}
		}
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.proposeBlock(ctx, w, genericBlock)
}

var jsonDecoders = map[string]blockDecoder{
	version.String(version.Fulu):      decodeFuluJSON,
	version.String(version.Electra):   decodeElectraJSON,
	version.String(version.Deneb):     decodeDenebJSON,
	version.String(version.Capella):   decodeCapellaJSON,
	version.String(version.Bellatrix): decodeBellatrixJSON,
	version.String(version.Altair):    decodeAltairJSON,
	version.String(version.Phase0):    decodePhase0JSON,
}

// decodeJSONToGenericBlock uses a lookup table to map a version string to the proper decoder.
func decodeJSONToGenericBlock(versionHeader string, body []byte) (*eth.GenericSignedBeaconBlock, error) {
	if decoder, found := jsonDecoders[versionHeader]; found {
		return decoder(body)
	}
	return nil, fmt.Errorf("body does not represent a valid block type")
}

func decodeFuluJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBeaconBlockContentsFulu](
		body,
		version.String(version.Fulu),
	)
}

func decodeElectraJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBeaconBlockContentsElectra](
		body,
		version.String(version.Electra),
	)
}

func decodeDenebJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBeaconBlockContentsDeneb](
		body,
		version.String(version.Deneb),
	)
}

func decodeCapellaJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBeaconBlockCapella](
		body,
		version.String(version.Capella),
	)
}

func decodeBellatrixJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBeaconBlockBellatrix](
		body,
		version.String(version.Bellatrix),
	)
}

func decodeAltairJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBeaconBlockAltair](
		body,
		version.String(version.Altair),
	)
}

func decodePhase0JSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	return decodeGenericJSON[*structs.SignedBeaconBlock](
		body,
		version.String(version.Phase0),
	)
}

// broadcastSidecarsIfSupported broadcasts blob sidecars when an equivocated block occurs.
func broadcastSidecarsIfSupported(ctx context.Context, s *Server, b interfaces.SignedBeaconBlock, gb *eth.GenericSignedBeaconBlock, versionHeader string) error {
	switch versionHeader {
	case version.String(version.Electra):
		return s.broadcastSeenBlockSidecars(ctx, b, gb.GetElectra().Blobs, gb.GetElectra().KzgProofs)
	case version.String(version.Deneb):
		return s.broadcastSeenBlockSidecars(ctx, b, gb.GetDeneb().Blobs, gb.GetDeneb().KzgProofs)
	default:
		// other forks before Deneb do not support blob sidecars
		// forks after fulu do not support blob sidecars, instead support data columns, no need to rebroadcast
		return nil
	}
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
		if err := s.validateConsensus(ctx, blk); err != nil {
			return errors.Wrap(err, "consensus validation failed")
		}
	case broadcastValidationConsensusAndEquivocation:
		if err := s.validateConsensus(r.Context(), blk); err != nil {
			return errors.Wrap(err, "consensus validation failed")
		}
		b, err := blocks.NewSignedBeaconBlock(blk.Block)
		if err != nil {
			return errors.Wrapf(err, "could not create signed beacon block")
		}
		if err = s.validateEquivocation(b.Block()); err != nil {
			return errors.Wrap(err, "equivocation validation failed")
		}
	default:
		return nil
	}
	return nil
}

func (s *Server) validateConsensus(ctx context.Context, b *eth.GenericSignedBeaconBlock) error {
	blk, err := blocks.NewSignedBeaconBlock(b.Block)
	if err != nil {
		return errors.Wrapf(err, "could not create signed beacon block")
	}

	parentBlockRoot := blk.Block().ParentRoot()
	parentBlock, err := s.Blocker.Block(ctx, parentBlockRoot[:])
	if err != nil {
		return errors.Wrap(err, "could not get parent block")
	}

	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		return errors.Wrap(err, "could not validate block")
	}

	parentStateRoot := parentBlock.Block().StateRoot()
	// Check if the state is already cached
	parentState := transition.NextSlotState(parentBlockRoot[:], blk.Block().Slot())
	if parentState == nil {
		// The state is not advanced in the NSC, check first if the parent post-state is head
		headRoot, err := s.HeadFetcher.HeadRoot(ctx)
		if err != nil {
			return errors.Wrap(err, "could not get head root")
		}
		if bytes.Equal(headRoot, parentBlockRoot[:]) {
			parentState, err = s.HeadFetcher.HeadState(ctx)
			if err != nil {
				return errors.Wrap(err, "could not get head state")
			}
			parentState, err = transition.ProcessSlots(ctx, parentState, blk.Block().Slot())
			if err != nil {
				return errors.Wrap(err, "could not process slots to get parent state")
			}
		} else {
			parentState, err = s.Stater.State(ctx, parentStateRoot[:])
			if err != nil {
				return errors.Wrap(err, "could not get parent state")
			}
		}
	}
	_, err = transition.ExecuteStateTransition(ctx, parentState, blk)
	if err != nil {
		return errors.Wrap(err, "could not execute state transition")
	}

	var blobs [][]byte
	var proofs [][]byte
	switch blk.Version() {
	case version.Deneb:
		blobs = b.GetDeneb().Blobs
		proofs = b.GetDeneb().KzgProofs
	case version.Electra:
		blobs = b.GetElectra().Blobs
		proofs = b.GetElectra().KzgProofs
	case version.Fulu:
		blobs = b.GetFulu().Blobs
		proofs = b.GetFulu().KzgProofs
	default:
		return nil
	}

	if err := s.validateBlobs(blk, blobs, proofs); err != nil {
		return err
	}

	return nil
}

func (s *Server) validateEquivocation(blk interfaces.ReadOnlyBeaconBlock) error {
	if s.ForkchoiceFetcher.HighestReceivedBlockSlot() == blk.Slot() {
		return errors.Wrapf(errEquivocatedBlock, "block for slot %d already exists in fork choice", blk.Slot())
	}
	return nil
}

func (s *Server) validateBlobs(blk interfaces.SignedBeaconBlock, blobs [][]byte, proofs [][]byte) error {
	if blk.Version() < version.Deneb {
		return nil
	}
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	commitments, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		return errors.Wrap(err, "could not get blob kzg commitments")
	}
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(blk.Block().Slot())
	if len(blobs) > maxBlobsPerBlock {
		return fmt.Errorf("number of blobs over max, %d > %d", len(blobs), maxBlobsPerBlock)
	}
	if blk.Version() >= version.Fulu {
		// For Fulu blocks, proofs are cell proofs (blobs * numberOfColumns)
		expectedProofsCount := uint64(len(blobs)) * numberOfColumns
		if uint64(len(proofs)) != expectedProofsCount || len(blobs) != len(commitments) {
			return fmt.Errorf("number of blobs (%d), cell proofs (%d), and commitments (%d) do not match (expected %d cell proofs)", len(blobs), len(proofs), len(commitments), expectedProofsCount)
		}
		// For Fulu blocks, proofs are cell proofs from execution client's BlobsBundleV2
		// Verify cell proofs directly without reconstructing data column sidecars
		if err := kzg.VerifyCellKZGProofBatchFromBlobData(blobs, commitments, proofs, numberOfColumns); err != nil {
			return errors.Wrap(err, "could not verify cell proofs")
		}
	} else {
		// For pre-Fulu blocks, proofs are blob proofs (1:1 with blobs)
		if len(blobs) != len(proofs) || len(blobs) != len(commitments) {
			return errors.Errorf("number of blobs (%d), proofs (%d), and commitments (%d) do not match", len(blobs), len(proofs), len(commitments))
		}
		// Use batch verification for better performance
		if err := kzg.VerifyBlobKZGProofBatch(blobs, commitments, proofs); err != nil {
			return errors.Wrap(err, "could not verify blob proofs")
		}
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
		helpers.HandleIsOptimisticError(w, err)
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

	// Validate that the provided slot belongs to the specified epoch, as required by the Beacon API spec.
	if rawSlot != "" {
		if sl < uint64(startSlot) || sl > uint64(endSlot) {
			httputil.HandleError(w, fmt.Sprintf("Slot %d does not belong in epoch %d", sl, epoch), http.StatusBadRequest)
			return
		}
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
		helpers.HandleIsOptimisticError(w, err)
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
	if !shared.WriteBlockFetchError(w, blk, err) {
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
		helpers.HandleIsOptimisticError(w, err)
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

// Deprecated: no longer needed post Electra
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
		httputil.WriteSsz(w, sszData)
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

	// Broadcast blob sidecars with forkchoice checking
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

// GetPendingConsolidations returns pending deposits for state with given 'stateId'.
// Should return 400 if the state retrieved is prior to Electra.
// Supports both JSON and SSZ responses based on Accept header.
func (s *Server) GetPendingConsolidations(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetPendingDeposits")
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
	if st.Version() < version.Electra {
		httputil.HandleError(w, "state_id is prior to electra", http.StatusBadRequest)
		return
	}
	pd, err := st.PendingConsolidations()
	if err != nil {
		httputil.HandleError(w, "Could not get pending consolidations: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(st.Version()))
	if httputil.RespondWithSsz(r) {
		sszData, err := serializeItems(pd)
		if err != nil {
			httputil.HandleError(w, "Failed to serialize pending consolidations: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszData)
	} else {
		isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
		if err != nil {
			helpers.HandleIsOptimisticError(w, err)
			return
		}
		blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
		if err != nil {
			httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
			return
		}
		isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)
		resp := structs.GetPendingConsolidationsResponse{
			Version:             version.String(st.Version()),
			ExecutionOptimistic: isOptimistic,
			Finalized:           isFinalized,
			Data:                structs.PendingConsolidationsFromConsensus(pd),
		}
		httputil.WriteJson(w, resp)
	}
}

// GetPendingDeposits returns pending deposits for state with given 'stateId'.
// Should return 400 if the state retrieved is prior to Electra.
// Supports both JSON and SSZ responses based on Accept header.
func (s *Server) GetPendingDeposits(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetPendingDeposits")
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
	if st.Version() < version.Electra {
		httputil.HandleError(w, "state_id is prior to electra", http.StatusBadRequest)
		return
	}
	pd, err := st.PendingDeposits()
	if err != nil {
		httputil.HandleError(w, "Could not get pending deposits: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(st.Version()))
	if httputil.RespondWithSsz(r) {
		sszData, err := serializeItems(pd)
		if err != nil {
			httputil.HandleError(w, "Failed to serialize pending deposits: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszData)
	} else {
		isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
		if err != nil {
			helpers.HandleIsOptimisticError(w, err)
			return
		}
		blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
		if err != nil {
			httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
			return
		}
		isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)
		resp := structs.GetPendingDepositsResponse{
			Version:             version.String(st.Version()),
			ExecutionOptimistic: isOptimistic,
			Finalized:           isFinalized,
			Data:                structs.PendingDepositsFromConsensus(pd),
		}
		httputil.WriteJson(w, resp)
	}
}

// GetPendingPartialWithdrawals returns pending partial withdrawals for state with given 'stateId'.
// Should return 400 if the state retrieved is prior to Electra.
// Supports both JSON and SSZ responses based on Accept header.
func (s *Server) GetPendingPartialWithdrawals(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetPendingPartialWithdrawals")
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
	if st.Version() < version.Electra {
		httputil.HandleError(w, "state_id is prior to electra", http.StatusBadRequest)
		return
	}
	ppw, err := st.PendingPartialWithdrawals()
	if err != nil {
		httputil.HandleError(w, "Could not get pending partial withdrawals: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(st.Version()))
	if httputil.RespondWithSsz(r) {
		sszData, err := serializeItems(ppw)
		if err != nil {
			httputil.HandleError(w, "Failed to serialize pending partial withdrawals: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszData)
	} else {
		isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
		if err != nil {
			helpers.HandleIsOptimisticError(w, err)
			return
		}
		blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
		if err != nil {
			httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
			return
		}
		isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)
		resp := structs.GetPendingPartialWithdrawalsResponse{
			Version:             version.String(st.Version()),
			ExecutionOptimistic: isOptimistic,
			Finalized:           isFinalized,
			Data:                structs.PendingPartialWithdrawalsFromConsensus(ppw),
		}
		httputil.WriteJson(w, resp)
	}
}

func (s *Server) GetProposerLookahead(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetProposerLookahead")
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
	if st.Version() < version.Fulu {
		httputil.HandleError(w, "state_id is prior to fulu", http.StatusBadRequest)
		return
	}
	pl, err := st.ProposerLookahead()
	if err != nil {
		httputil.HandleError(w, "Could not get proposer look ahead: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(st.Version()))
	if httputil.RespondWithSsz(r) {
		sszLen := (*primitives.ValidatorIndex)(nil).SizeSSZ()
		sszData := make([]byte, len(pl)*sszLen)
		for i, idx := range pl {
			copy(sszData[i*sszLen:(i+1)*sszLen], ssz.MarshalUint64([]byte{}, uint64(idx)))
		}
		httputil.WriteSsz(w, sszData)
	} else {
		isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
		if err != nil {
			helpers.HandleIsOptimisticError(w, err)
			return
		}
		blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
		if err != nil {
			httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
			return
		}
		isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)
		vi := make([]string, len(pl))
		for i, v := range pl {
			vi[i] = strconv.FormatUint(uint64(v), 10)
		}
		resp := structs.GetProposerLookaheadResponse{
			Version:             version.String(st.Version()),
			ExecutionOptimistic: isOptimistic,
			Finalized:           isFinalized,
			Data:                vi,
		}
		httputil.WriteJson(w, resp)
	}
}

// SerializeItems serializes a slice of items, each of which implements the MarshalSSZ method,
// into a single byte array.
func serializeItems[T interface{ MarshalSSZ() ([]byte, error) }](items []T) ([]byte, error) {
	var result []byte
	for _, item := range items {
		b, err := item.MarshalSSZ()
		if err != nil {
			return nil, err
		}
		result = append(result, b...)
	}
	return result, nil
}
