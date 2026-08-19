package execution

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

var (
	supportedEngineEndpoints = []string{
		NewPayloadMethod,
		NewPayloadMethodV2,
		NewPayloadMethodV3,
		ForkchoiceUpdatedMethod,
		ForkchoiceUpdatedMethodV2,
		ForkchoiceUpdatedMethodV3,
		GetPayloadMethod,
		GetPayloadMethodV2,
		GetPayloadMethodV3,
		GetPayloadBodiesByHashV1,
		GetPayloadBodiesByRangeV1,
		GetBlobsV1,
	}

	electraEngineEndpoints = []string{
		NewPayloadMethodV4,
		GetPayloadMethodV4,
	}

	fuluEngineEndpoints = []string{
		GetPayloadMethodV5,
		GetBlobsV2,
		GetBlobsV3,
		HasBlobs,
	}

	gloasEngineEndpoints = []string{
		NewPayloadMethodV5,
		GetPayloadMethodV6,
		ForkchoiceUpdatedMethodV4,
		GetPayloadBodiesByHashV2,
		GetPayloadBodiesByRangeV2,
	}
)

const (
	// NewPayloadMethod v1 request string for JSON-RPC.
	NewPayloadMethod = "engine_newPayloadV1"
	// NewPayloadMethodV2 v2 request string for JSON-RPC.
	NewPayloadMethodV2 = "engine_newPayloadV2"
	NewPayloadMethodV3 = "engine_newPayloadV3"
	// NewPayloadMethodV4 is the engine_newPayloadVX method added at Electra.
	NewPayloadMethodV4 = "engine_newPayloadV4"
	// NewPayloadMethodV5 is the engine_newPayloadVX method added at Gloas.
	NewPayloadMethodV5 = "engine_newPayloadV5"
	// ForkchoiceUpdatedMethod v1 request string for JSON-RPC.
	ForkchoiceUpdatedMethod = "engine_forkchoiceUpdatedV1"
	// ForkchoiceUpdatedMethodV2 v2 request string for JSON-RPC.
	ForkchoiceUpdatedMethodV2 = "engine_forkchoiceUpdatedV2"
	// ForkchoiceUpdatedMethodV3 v3 request string for JSON-RPC.
	ForkchoiceUpdatedMethodV3 = "engine_forkchoiceUpdatedV3"
	// GetPayloadMethod v1 request string for JSON-RPC.
	GetPayloadMethod = "engine_getPayloadV1"
	// GetPayloadMethodV2 v2 request string for JSON-RPC.
	GetPayloadMethodV2 = "engine_getPayloadV2"
	// GetPayloadMethodV3 is the get payload method added for deneb
	GetPayloadMethodV3 = "engine_getPayloadV3"
	// GetPayloadMethodV4 is the get payload method added for electra
	GetPayloadMethodV4 = "engine_getPayloadV4"
	// GetPayloadMethodV5 is the get payload method added for fulu
	GetPayloadMethodV5 = "engine_getPayloadV5"
	// GetPayloadMethodV6 is the get payload method added for gloas/amsterdam.
	GetPayloadMethodV6 = "engine_getPayloadV6"
	// ForkchoiceUpdatedMethodV4 is the forkchoice updated method added for gloas/amsterdam.
	ForkchoiceUpdatedMethodV4 = "engine_forkchoiceUpdatedV4"
	// BlockByHashMethod request string for JSON-RPC.
	BlockByHashMethod = "eth_getBlockByHash"
	// BlockByNumberMethod request string for JSON-RPC.
	BlockByNumberMethod = "eth_getBlockByNumber"
	// GetPayloadBodiesByHashV1 is the engine_getPayloadBodiesByHashX JSON-RPC method for pre-Electra payloads.
	GetPayloadBodiesByHashV1 = "engine_getPayloadBodiesByHashV1"
	// GetPayloadBodiesByRangeV1 is the engine_getPayloadBodiesByRangeX JSON-RPC method for pre-Electra payloads.
	GetPayloadBodiesByRangeV1 = "engine_getPayloadBodiesByRangeV1"
	// GetPayloadBodiesByHashV2 is the engine_getPayloadBodiesByHashV2 JSON-RPC method for amsterdam payloads.
	GetPayloadBodiesByHashV2 = "engine_getPayloadBodiesByHashV2"
	// GetPayloadBodiesByRangeV2 is the engine_getPayloadBodiesByRangeV2 JSON-RPC method for amsterdam payloads.
	GetPayloadBodiesByRangeV2 = "engine_getPayloadBodiesByRangeV2"
	// ExchangeCapabilities request string for JSON-RPC.
	ExchangeCapabilities = "engine_exchangeCapabilities"
	// GetBlobsV1 request string for JSON-RPC.
	GetBlobsV1 = "engine_getBlobsV1"
	// GetBlobsV2 request string for JSON-RPC.
	GetBlobsV2 = "engine_getBlobsV2"
	// GetBlobsV3 request string for JSON-RPC.
	GetBlobsV3 = "engine_getBlobsV3"
	// HasBlobs request string for JSON-RPC.
	HasBlobs = "engine_hasBlobs"
	// GetBlobsV4 request string for JSON-RPC (EIP-8070 sparse blobpool).
	GetBlobsV4 = "engine_getBlobsV4"
	// GetClientVersionV1 is the JSON-RPC method that identifies the execution client.
	GetClientVersionV1 = "engine_getClientVersionV1"
	// Defines the seconds before timing out engine endpoints with non-block execution semantics.
	defaultEngineTimeout = time.Second
	// gloasGetPayloadTimeout is the maximum time allowed for engine_getPayloadV6.
	gloasGetPayloadTimeout = 300 * time.Millisecond
)

// NewPayload request calls the engine_newPayloadVX method via JSON-RPC.
func (s *Service) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests pb.ExecutionRequester) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.NewPayload")
	defer span.End()
	defer func(start time.Time) {
		newPayloadLatency.Observe(float64(time.Since(start).Milliseconds()))
	}(time.Now())

	d := time.Now().Add(time.Duration(params.BeaconConfig().ExecutionEngineTimeoutValue) * time.Second)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()
	result := &pb.PayloadStatus{}

	switch payloadPb := payload.Proto().(type) {
	case *pb.ExecutionPayload:
		err := s.rpcClient.CallContext(ctx, result, NewPayloadMethod, payloadPb)
		if err != nil {
			return nil, handleRPCError(err)
		}
	case *pb.ExecutionPayloadCapella:
		err := s.rpcClient.CallContext(ctx, result, NewPayloadMethodV2, payloadPb)
		if err != nil {
			return nil, handleRPCError(err)
		}
	case *pb.ExecutionPayloadDeneb:
		if executionRequests == nil {
			err := s.rpcClient.CallContext(ctx, result, NewPayloadMethodV3, payloadPb, versionedHashes, parentBlockRoot)
			if err != nil {
				return nil, handleRPCError(err)
			}
		} else {
			flattenedRequests, err := executionRequests.FlattenRequests()
			if err != nil {
				return nil, errors.Wrap(err, "failed to encode execution requests")
			}
			err = s.rpcClient.CallContext(ctx, result, NewPayloadMethodV4, payloadPb, versionedHashes, parentBlockRoot, flattenedRequests)
			if err != nil {
				return nil, handleRPCError(err)
			}
		}
	case *pb.ExecutionPayloadGloas:
		flattenedRequests, err := executionRequests.FlattenRequests()
		if err != nil {
			return nil, errors.Wrap(err, "failed to encode execution requests")
		}
		err = s.rpcClient.CallContext(ctx, result, NewPayloadMethodV5, payloadPb, versionedHashes, parentBlockRoot, flattenedRequests)
		if err != nil {
			return nil, handleRPCError(err)
		}
	default:
		return nil, errors.New("unknown execution data type")
	}
	if result.ValidationError != "" {
		log.WithField("status", result.Status.String()).
			WithField("parentRoot", fmt.Sprintf("%#x", parentBlockRoot)).
			WithError(errors.New(result.ValidationError)).
			Error("Got a validation error in newPayload")
	}
	switch result.Status {
	case pb.PayloadStatus_INVALID_BLOCK_HASH:
		return nil, ErrInvalidBlockHashPayloadStatus
	case pb.PayloadStatus_ACCEPTED, pb.PayloadStatus_SYNCING:
		return nil, ErrAcceptedSyncingPayloadStatus
	case pb.PayloadStatus_INVALID:
		return result.LatestValidHash, ErrInvalidPayloadStatus
	case pb.PayloadStatus_VALID:
		return result.LatestValidHash, nil
	default:
		return nil, errors.Wrapf(ErrUnknownPayloadStatus, "unknown payload status: %s", result.Status.String())
	}
}

// ForkchoiceUpdatedResponse is the response kind received by the
// engine_forkchoiceUpdatedV1 endpoint.
type ForkchoiceUpdatedResponse struct {
	Status          *pb.PayloadStatus  `json:"payloadStatus"`
	PayloadId       *pb.PayloadIDBytes `json:"payloadId"`
	ValidationError string             `json:"validationError"`
}

// ForkchoiceUpdated calls the engine_forkchoiceUpdatedV1 method via JSON-RPC.
// custodyColumns is only sent on engine_forkchoiceUpdatedV4 (EIP-8070), and only when the sparse
// blobpool is enabled and the execution client supports it; a nil/empty set omits the parameter,
// which the execution client treats as "keep the current custody set".
func (s *Service) ForkchoiceUpdated(
	ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer, custodyColumns map[uint64]bool,
) (*pb.PayloadIDBytes, []byte, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ForkchoiceUpdated")
	defer span.End()
	start := time.Now()
	defer func() {
		forkchoiceUpdatedLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()

	d := time.Now().Add(time.Duration(params.BeaconConfig().ExecutionEngineTimeoutValue) * time.Second)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()
	result := &ForkchoiceUpdatedResponse{}

	if attrs == nil {
		return nil, nil, errors.New("nil payload attributer")
	}
	switch attrs.Version() {
	case version.Bellatrix:
		a, err := attrs.PbV1()
		if err != nil {
			return nil, nil, err
		}
		err = s.rpcClient.CallContext(ctx, result, ForkchoiceUpdatedMethod, state, a)
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	case version.Capella:
		a, err := attrs.PbV2()
		if err != nil {
			return nil, nil, err
		}
		err = s.rpcClient.CallContext(ctx, result, ForkchoiceUpdatedMethodV2, state, a)
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	case version.Deneb, version.Electra, version.Fulu:
		a, err := attrs.PbV3()
		if err != nil {
			return nil, nil, err
		}
		err = s.rpcClient.CallContext(ctx, result, ForkchoiceUpdatedMethodV3, state, a)
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	case version.Gloas:
		a, err := attrs.PbV4()
		if err != nil {
			return nil, nil, err
		}
		// An all-zero mask would contract the EL's custody set to nothing, and a non-EIP-8070 EL
		// rejects the extra parameter, so it is omitted unless there is a set to send.
		if s.sendCustodyColumns() && len(custodyColumns) > 0 {
			mask := custodyColumnsBitmask(custodyColumns)
			log.WithFields(logrus.Fields{
				"columns": mask.Count(),
				"eager":   mask.Count() >= fieldparams.CellsPerBlob,
			}).Debug("Sent custody columns on forkchoiceUpdatedV4")
			err = s.rpcClient.CallContext(ctx, result, ForkchoiceUpdatedMethodV4, state, a, hexutil.Bytes(mask))
		} else {
			err = s.rpcClient.CallContext(ctx, result, ForkchoiceUpdatedMethodV4, state, a)
		}
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	default:
		return nil, nil, fmt.Errorf("unknown payload attribute version: %v", attrs.Version())
	}

	if result.Status == nil {
		return nil, nil, ErrNilResponse
	}
	if result.ValidationError != "" {
		log.WithError(errors.New(result.ValidationError)).Error("Got a validation error in forkChoiceUpdated")
	}
	resp := result.Status
	switch resp.Status {
	case pb.PayloadStatus_SYNCING:
		return nil, nil, ErrAcceptedSyncingPayloadStatus
	case pb.PayloadStatus_INVALID:
		return nil, resp.LatestValidHash, ErrInvalidPayloadStatus
	case pb.PayloadStatus_VALID:
		return result.PayloadId, resp.LatestValidHash, nil
	default:
		return nil, nil, ErrUnknownPayloadStatus
	}
}

func getPayloadMethodAndMessage(slot primitives.Slot) (string, proto.Message) {
	epoch := slots.ToEpoch(slot)
	if epoch >= params.BeaconConfig().GloasForkEpoch {
		return GetPayloadMethodV6, &pb.ExecutionBundleGloas{}
	}
	if epoch >= params.BeaconConfig().FuluForkEpoch {
		return GetPayloadMethodV5, &pb.ExecutionBundleFulu{}
	}
	if epoch >= params.BeaconConfig().ElectraForkEpoch {
		return GetPayloadMethodV4, &pb.ExecutionBundleElectra{}
	}
	if epoch >= params.BeaconConfig().DenebForkEpoch {
		return GetPayloadMethodV3, &pb.ExecutionPayloadDenebWithValueAndBlobsBundle{}
	}
	if epoch >= params.BeaconConfig().CapellaForkEpoch {
		return GetPayloadMethodV2, &pb.ExecutionPayloadCapellaWithValue{}
	}
	return GetPayloadMethod, &pb.ExecutionPayload{}
}

func getPayloadTimeout(slot primitives.Slot) time.Duration {
	if slots.ToEpoch(slot) >= params.BeaconConfig().GloasForkEpoch {
		return gloasGetPayloadTimeout
	}
	return defaultEngineTimeout
}

// GetPayload calls the engine_getPayloadVX method via JSON-RPC.
// It returns the execution data as well as the blobs bundle.
func (s *Service) GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetPayload")
	defer span.End()
	start := time.Now()
	defer func() {
		getPayloadLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()
	d := time.Now().Add(getPayloadTimeout(slot))
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()

	method, result := getPayloadMethodAndMessage(slot)
	err := s.rpcClient.CallContext(ctx, result, method, pb.PayloadIDBytes(payloadId))
	if err != nil {
		return nil, handleRPCError(err)
	}
	res, err := blocks.NewGetPayloadResponse(result)
	if err != nil {
		return nil, errors.Wrap(err, "new get payload response")
	}
	return res, nil
}

func (s *Service) ExchangeCapabilities(ctx context.Context) ([]string, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ExchangeCapabilities")
	defer span.End()

	capacity := len(supportedEngineEndpoints)
	if params.ElectraEnabled() {
		capacity += len(electraEngineEndpoints)
	}
	if params.FuluEnabled() {
		capacity += len(fuluEngineEndpoints)
	}
	if params.GloasEnabled() {
		capacity += len(gloasEngineEndpoints)
	}

	endpoints := make([]string, 0, capacity+1)
	endpoints = append(endpoints, supportedEngineEndpoints...)
	if params.ElectraEnabled() {
		endpoints = append(endpoints, electraEngineEndpoints...)
	}
	if params.FuluEnabled() {
		endpoints = append(endpoints, fuluEngineEndpoints...)
	}
	if params.GloasEnabled() {
		endpoints = append(endpoints, gloasEngineEndpoints...)
	}
	// Advertising engine_getBlobsV4 switches an EIP-8070 EL's blobpool into cell mode, so it is
	// only advertised when the sparse blobpool is explicitly enabled.
	if s.advertiseGetBlobsV4() {
		endpoints = append(endpoints, GetBlobsV4)
	}

	elSupportedEndpointsSlice := make([]string, 0, len(endpoints))
	if err := s.rpcClient.CallContext(ctx, &elSupportedEndpointsSlice, ExchangeCapabilities, endpoints); err != nil {
		return nil, handleRPCError(err)
	}

	elSupportedEndpoints := make(map[string]bool, len(elSupportedEndpointsSlice))
	for _, method := range elSupportedEndpointsSlice {
		elSupportedEndpoints[method] = true
	}

	unsupported := make([]string, 0)
	for _, method := range endpoints {
		if !elSupportedEndpoints[method] {
			unsupported = append(unsupported, method)
		}
	}

	if len(unsupported) != 0 {
		log.WithField("methods", unsupported).Warning("Connected execution client does not support some requested engine methods")
	}

	return elSupportedEndpointsSlice, nil
}

// GetClientVersion calls engine_getClientVersionV1 to retrieve EL client information.
func (s *Service) GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetClientVersionV1")
	defer span.End()

	// First 4 bytes of the git commit are used.
	commit := version.GitCommit()
	if len(commit) >= 8 {
		commit = commit[:8]
	}

	var result []*structs.ClientVersionV1
	err := s.rpcClient.CallContext(
		ctx,
		&result,
		GetClientVersionV1,
		structs.ClientVersionV1{
			Code:    PrysmClientCode,
			Name:    PrysmClientName,
			Version: version.SemanticVersion(),
			Commit:  commit,
		},
	)
	if err != nil {
		return nil, handleRPCError(err)
	}

	if len(result) == 0 {
		return nil, errors.New("execution client returned no result")
	}

	return result, nil
}

// GetBlobs returns the blob and proof from the execution engine for the given versioned hashes.
func (s *Service) GetBlobs(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetBlobs")
	defer span.End()

	// If the execution engine does not support `GetBlobsV1`, return early to prevent encountering an error later.
	if !s.capabilityCache.has(GetBlobsV1) {
		return nil, errors.New(fmt.Sprintf("%s is not supported", GetBlobsV1))
	}

	result := make([]*pb.BlobAndProof, len(versionedHashes))
	err := s.rpcClient.CallContext(ctx, &result, GetBlobsV1, versionedHashes)
	return result, handleRPCError(err)
}

func (s *Service) GetBlobsV2(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetBlobsV2")
	defer span.End()

	start := time.Now()

	if !s.capabilityCache.has(GetBlobsV2) {
		return nil, errors.New(fmt.Sprintf("%s is not supported", GetBlobsV2))
	}

	if flags.Get().DisableGetBlobsV2 {
		return []*pb.BlobAndProofV2{}, nil
	}

	result := make([]*pb.BlobAndProofV2, len(versionedHashes))
	err := s.rpcClient.CallContext(ctx, &result, GetBlobsV2, versionedHashes)

	if len(result) != 0 {
		getBlobsV2Latency.Observe(float64(time.Since(start).Milliseconds()))
	}

	return result, handleRPCError(err)
}

func (s *Service) GetBlobsV3(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetBlobsV3")
	defer span.End()
	start := time.Now()

	getBlobsV3RequestsTotal.Inc()
	result := make([]*pb.BlobAndProofV2, len(versionedHashes))
	if err := s.rpcClient.CallContext(ctx, &result, GetBlobsV3, versionedHashes); err != nil {
		return nil, handleRPCError(err)
	}
	getBlobsV3Latency.Observe(float64(time.Since(start).Seconds()))
	return result, nil
}

// HasBlobs checks whether the given versioned hashes are available in the
// execution client's blob pool without fetching the actual blob data.
// It returns a boolean slice parallel to the input hashes.
func (s *Service) HasBlobs(ctx context.Context, versionedHashes []common.Hash) ([]bool, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.HasBlobs")
	defer span.End()
	start := time.Now()

	hasBlobsRequestsTotal.Inc()
	var result []bool
	if err := s.rpcClient.CallContext(ctx, &result, HasBlobs, versionedHashes); err != nil {
		return nil, handleRPCError(err)
	}
	hasBlobsLatency.Observe(float64(time.Since(start).Seconds()))
	return result, nil
}

// GetBlobsV4 calls the engine_getBlobsV4 method via JSON-RPC (EIP-8070).
// It fetches the blob cells and KZG proofs at the given cell indices for the given versioned
// hashes. The response is dense: each blob's cells/proofs arrays are parallel to the ascending
// set bits of indicesBitarray, with null entries for cells the EL does not have.
func (s *Service) GetBlobsV4(ctx context.Context, versionedHashes []common.Hash, indicesBitarray []byte) ([]*pb.BlobCellsAndProofsV1, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetBlobsV4")
	defer span.End()
	start := time.Now()

	if !s.capabilityCache.has(GetBlobsV4) {
		return nil, errors.Errorf("%s is not supported", GetBlobsV4)
	}

	getBlobsV4RequestsTotal.Inc()
	var result []*pb.BlobCellsAndProofsV1
	if err := s.rpcClient.CallContext(ctx, &result, GetBlobsV4, versionedHashes, hexutil.Bytes(indicesBitarray)); err != nil {
		return nil, handleRPCError(err)
	}
	getBlobsV4Latency.Observe(float64(time.Since(start).Seconds()))
	return result, nil
}

// advertiseGetBlobsV4 reports whether engine_getBlobsV4 is advertised to the execution client in
// engine_exchangeCapabilities. Advertising it flips an EIP-8070 EL's blobpool into cell mode.
func (s *Service) advertiseGetBlobsV4() bool {
	return s.sparseBlobpoolEnabled && params.GloasEnabled()
}

// sendCustodyColumns reports whether the custody columns parameter is attached to
// engine_forkchoiceUpdatedV4 calls. The EL advertising engine_getBlobsV4 is used as the signal
// that it implements EIP-8070; a non-EIP-8070 EL would reject the extra parameter.
func (s *Service) sendCustodyColumns() bool {
	return s.sparseBlobpoolEnabled && s.capabilityCache.has(GetBlobsV4)
}

// custodyColumnsBitmask encodes the custody column set as the 16-byte bitarray expected by
// engine_forkchoiceUpdatedV4 and engine_getBlobsV4.
func custodyColumnsBitmask(custodyColumns map[uint64]bool) bitfield.Bitvector128 {
	mask := bitfield.NewBitvector128()
	for col, ok := range custodyColumns {
		if ok {
			mask.SetBitAt(col, true)
		}
	}
	return mask
}
