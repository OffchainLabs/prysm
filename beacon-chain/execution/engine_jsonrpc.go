package execution

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
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
	// GetClientVersionV1 is the JSON-RPC method that identifies the execution client.
	GetClientVersionV1 = "engine_getClientVersionV1"
	// Defines the seconds before timing out engine endpoints with non-block execution semantics.
	defaultEngineTimeout = time.Second
)

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

// jsonEngine is the JSON-RPC (engine_*) implementation of engineTransport. It
// holds only its wire dependencies — the shared JSON-RPC connection and the
// engine-capability cache, injected by engine() — mirroring sszEngine (which
// holds its enginehttp client). It does not reach back into Service.
type jsonEngine struct {
	rpc  RPCClient
	caps *capabilityCache
}

// NewPayload calls the engine_newPayloadVX method via JSON-RPC.
func (j jsonEngine) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests *pb.ExecutionRequests) ([]byte, error) {
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
		err := j.rpc.CallContext(ctx, result, NewPayloadMethod, payloadPb)
		if err != nil {
			return nil, handleRPCError(err)
		}
	case *pb.ExecutionPayloadCapella:
		err := j.rpc.CallContext(ctx, result, NewPayloadMethodV2, payloadPb)
		if err != nil {
			return nil, handleRPCError(err)
		}
	case *pb.ExecutionPayloadDeneb:
		if executionRequests == nil {
			err := j.rpc.CallContext(ctx, result, NewPayloadMethodV3, payloadPb, versionedHashes, parentBlockRoot)
			if err != nil {
				return nil, handleRPCError(err)
			}
		} else {
			flattenedRequests, err := pb.EncodeExecutionRequests(executionRequests)
			if err != nil {
				return nil, errors.Wrap(err, "failed to encode execution requests")
			}
			err = j.rpc.CallContext(ctx, result, NewPayloadMethodV4, payloadPb, versionedHashes, parentBlockRoot, flattenedRequests)
			if err != nil {
				return nil, handleRPCError(err)
			}
		}
	case *pb.ExecutionPayloadGloas:
		flattenedRequests, err := pb.EncodeExecutionRequests(executionRequests)
		if err != nil {
			return nil, errors.Wrap(err, "failed to encode execution requests")
		}
		err = j.rpc.CallContext(ctx, result, NewPayloadMethodV5, payloadPb, versionedHashes, parentBlockRoot, flattenedRequests)
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
func (j jsonEngine) ForkchoiceUpdated(
	ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer,
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
		err = j.rpc.CallContext(ctx, result, ForkchoiceUpdatedMethod, state, a)
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	case version.Capella:
		a, err := attrs.PbV2()
		if err != nil {
			return nil, nil, err
		}
		err = j.rpc.CallContext(ctx, result, ForkchoiceUpdatedMethodV2, state, a)
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	case version.Deneb, version.Electra, version.Fulu:
		a, err := attrs.PbV3()
		if err != nil {
			return nil, nil, err
		}
		err = j.rpc.CallContext(ctx, result, ForkchoiceUpdatedMethodV3, state, a)
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	case version.Gloas:
		a, err := attrs.PbV4()
		if err != nil {
			return nil, nil, err
		}
		err = j.rpc.CallContext(ctx, result, ForkchoiceUpdatedMethodV4, state, a)
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

// GetPayload calls the engine_getPayloadVX method via JSON-RPC.
// It returns the execution data as well as the blobs bundle.
func (j jsonEngine) GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetPayload")
	defer span.End()
	start := time.Now()
	defer func() {
		getPayloadLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()
	d := time.Now().Add(defaultEngineTimeout)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()

	method, result := getPayloadMethodAndMessage(slot)
	err := j.rpc.CallContext(ctx, result, method, pb.PayloadIDBytes(payloadId))
	if err != nil {
		return nil, handleRPCError(err)
	}
	res, err := blocks.NewGetPayloadResponse(result)
	if err != nil {
		return nil, errors.Wrap(err, "new get payload response")
	}
	return res, nil
}

func (j jsonEngine) ExchangeCapabilities(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ExchangeCapabilities")
	defer span.End()

	if params.ElectraEnabled() {
		supportedEngineEndpoints = append(supportedEngineEndpoints, electraEngineEndpoints...)
	}

	if params.FuluEnabled() {
		supportedEngineEndpoints = append(supportedEngineEndpoints, fuluEngineEndpoints...)
	}

	if params.GloasEnabled() {
		supportedEngineEndpoints = append(supportedEngineEndpoints, gloasEngineEndpoints...)
	}

	elSupportedEndpointsSlice := make([]string, len(supportedEngineEndpoints))
	if err := j.rpc.CallContext(ctx, &elSupportedEndpointsSlice, ExchangeCapabilities, supportedEngineEndpoints); err != nil {
		return handleRPCError(err)
	}

	elSupportedEndpoints := make(map[string]bool, len(elSupportedEndpointsSlice))
	for _, method := range elSupportedEndpointsSlice {
		elSupportedEndpoints[method] = true
	}

	unsupported := make([]string, 0)
	for _, method := range supportedEngineEndpoints {
		if !elSupportedEndpoints[method] {
			unsupported = append(unsupported, method)
		}
	}

	if len(unsupported) != 0 {
		log.WithField("methods", unsupported).Warning("Connected execution client does not support some requested engine methods")
	}

	// Cache the supported endpoints.
	j.caps.save(elSupportedEndpointsSlice)

	return nil
}

// GetBlobs returns the blob and proof from the execution engine for the given versioned hashes.
func (j jsonEngine) GetBlobs(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetBlobs")
	defer span.End()

	// If the execution engine does not support `GetBlobsV1`, return early to prevent encountering an error later.
	if !j.caps.has(GetBlobsV1) {
		return nil, errors.New(fmt.Sprintf("%s is not supported", GetBlobsV1))
	}

	result := make([]*pb.BlobAndProof, len(versionedHashes))
	err := j.rpc.CallContext(ctx, &result, GetBlobsV1, versionedHashes)
	return result, handleRPCError(err)
}

func (j jsonEngine) GetBlobsV2(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetBlobsV2")
	defer span.End()

	start := time.Now()

	if !j.caps.has(GetBlobsV2) {
		return nil, errors.New(fmt.Sprintf("%s is not supported", GetBlobsV2))
	}

	if flags.Get().DisableGetBlobsV2 {
		return []*pb.BlobAndProofV2{}, nil
	}

	result := make([]*pb.BlobAndProofV2, len(versionedHashes))
	err := j.rpc.CallContext(ctx, &result, GetBlobsV2, versionedHashes)

	if len(result) != 0 {
		getBlobsV2Latency.Observe(float64(time.Since(start).Milliseconds()))
	}

	return result, handleRPCError(err)
}

// PayloadBodyFork keys every block into a single V1 bodies batch, preserving
// the pre-transport behavior (one engine_getPayloadBodiesByHashV1 call). The
// fork is irrelevant to JSON-RPC, which is version- not URL-scoped.
func (j jsonEngine) PayloadBodyFork(int) string {
	return GetPayloadBodiesByHashV1
}

// GetPayloadBodiesByHash calls engine_getPayloadBodiesByHashV1. The result is
// aligned with hashes (nil for unavailable bodies). The error is returned raw,
// matching the prior reconstruction path.
func (j jsonEngine) GetPayloadBodiesByHash(ctx context.Context, _ string, hashes []common.Hash) ([]*pb.ExecutionPayloadBody, error) {
	result := make([]*pb.ExecutionPayloadBody, 0)
	if err := j.rpc.CallContext(ctx, &result, GetPayloadBodiesByHashV1, hashes); err != nil {
		return nil, err
	}
	return result, nil
}

// GetPayloadBodiesByRange calls engine_getPayloadBodiesByRangeV1.
func (j jsonEngine) GetPayloadBodiesByRange(ctx context.Context, _ string, from, count uint64) ([]*pb.ExecutionPayloadBody, error) {
	result := make([]*pb.ExecutionPayloadBody, 0)
	if err := j.rpc.CallContext(ctx, &result, GetPayloadBodiesByRangeV1, hexutil.EncodeUint64(from), hexutil.EncodeUint64(count)); err != nil {
		return nil, err
	}
	return result, nil
}

// GetClientVersion calls engine_getClientVersionV1 to retrieve EL client information.
func (j jsonEngine) GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetClientVersionV1")
	defer span.End()

	// First 4 bytes of the git commit are used.
	commit := version.GitCommit()
	if len(commit) >= 8 {
		commit = commit[:8]
	}

	var result []*structs.ClientVersionV1
	err := j.rpc.CallContext(
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

type capabilityCache struct {
	capabilities     map[string]any
	capabilitiesLock sync.RWMutex
}

func (c *capabilityCache) save(cs []string) {
	c.capabilitiesLock.Lock()
	defer c.capabilitiesLock.Unlock()

	if c.capabilities == nil {
		c.capabilities = make(map[string]any)
	}

	for _, capability := range cs {
		c.capabilities[capability] = struct{}{}
	}
}

func (c *capabilityCache) has(capability string) bool {
	c.capabilitiesLock.RLock()
	defer c.capabilitiesLock.RUnlock()

	if c.capabilities == nil {
		return false
	}

	_, ok := c.capabilities[capability]
	return ok
}
