package execution

import (
	"context"
	"slices"
	"sync"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution/enginehttp"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"google.golang.org/protobuf/proto"
)

// sszEngine implements engineTransport over REST + SSZ.
type sszEngine struct {
	client *enginehttp.Client

	capsLock sync.RWMutex
	caps     *enginehttp.Capabilities

	// fcuMu serializes forkchoice requests on a connection.
	fcuMu sync.Mutex
}

// Body-size directions for the engine_body_size_bytes metric (metrics.go).
const (
	directionRequest  = "request"
	directionResponse = "response"
)

// observeSSZBody records the SSZ wire size for one request or response body.
func observeSSZBody(method, direction string, m ssz.Marshaler) {
	engineBodySize.WithLabelValues(method, transportSSZ, direction).Observe(float64(m.SizeSSZ()))
}

// Keys in GET /engine/v1/capabilities "limits".
const (
	limitBodiesMaxCount          = "bodies.max_count"
	limitBlobsMaxVersionedHashes = "blobs.max_versioned_hashes"
	limitPayloadMaxBytes         = "payload.max_bytes"
)

// limit returns the advertised non-zero limit for key.
func (e *sszEngine) limit(key string) (uint64, bool) {
	e.capsLock.RLock()
	defer e.capsLock.RUnlock()
	if e.caps == nil {
		return 0, false
	}
	v, ok := e.caps.Limits[key]
	if !ok || v == 0 {
		return 0, false
	}
	return v, true
}

// rejectIfOverLimit applies a client-side capability limit.
func (e *sszEngine) rejectIfOverLimit(key string, n uint64) error {
	if maxN, ok := e.limit(key); ok && n > maxN {
		return errors.Wrapf(ErrRequestTooLarge, "request of %d exceeds advertised %s=%d", n, key, maxN)
	}
	return nil
}

// NewPayload submits a payload over POST /engine/v1/payloads.
func (e *sszEngine) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests pb.ExecutionRequester) ([]byte, error) {
	var (
		ver        int
		envelope   ssz.Marshaler
		parentRoot []byte
	)

	if parentBlockRoot != nil {
		parentRoot = parentBlockRoot[:]
	}

	switch p := payload.Proto().(type) {
	case *pb.ExecutionPayloadDeneb:
		envelope = &enginev2.ExecutionPayloadEnvelopeFulu{
			Payload:               p,
			ParentBeaconBlockRoot: parentRoot,
		}

		if executionRequests != nil {
			reqs, err := encodeExecutionRequests(executionRequests)
			if err != nil {
				return nil, err
			}
			envelope.(*enginev2.ExecutionPayloadEnvelopeFulu).ExecutionRequests = reqs
		}

		ver = version.Fulu
	case *pb.ExecutionPayloadGloas:
		reqs, err := encodeExecutionRequests(executionRequests)
		if err != nil {
			return nil, err
		}
		envelope = &enginev2.ExecutionPayloadEnvelopeGloas{
			Payload:               p,
			ParentBeaconBlockRoot: parentRoot,
			ExecutionRequests:     reqs,
		}

		ver = version.Gloas
	default:
		return nil, errors.Errorf("ssz-http engine transport: no ExecutionPayloadEnvelope container for payload type %T", p)
	}

	if err := e.rejectIfUnsupportedFork(ver); err != nil {
		return nil, err
	}
	if err := e.rejectIfOverLimit(limitPayloadMaxBytes, uint64(envelope.SizeSSZ())); err != nil {
		return nil, err
	}
	observeSSZBody(methodNewPayload, directionRequest, envelope)
	status, err := e.client.NewPayload(ctx, ver, envelope)
	if err != nil {
		return nil, err
	}
	observeSSZBody(methodNewPayload, directionResponse, status)
	return payloadStatusResult(status)
}

// payloadStatusResult maps SSZ PayloadStatus onto the JSON-RPC transport contract.
func payloadStatusResult(s *enginev2.PayloadStatus) ([]byte, error) {
	lvh, _ := enginev2.OptionalBytes(s.LatestValidHash)
	if valErr, ok := enginev2.OptionalBytes(s.ValidationError); ok && len(valErr) > 0 {
		log.WithError(errors.New(string(valErr))).Error("Got a validation error in newPayload")
	}
	switch s.Enum() {
	case enginev2.PayloadStatusAccepted, enginev2.PayloadStatusSyncing:
		return nil, ErrAcceptedSyncingPayloadStatus
	case enginev2.PayloadStatusInvalid:
		return lvh, ErrInvalidPayloadStatus
	case enginev2.PayloadStatusValid:
		return lvh, nil
	default:
		return nil, errors.Wrapf(ErrUnknownPayloadStatus, "unknown payload status: %d", s.Enum())
	}
}

// encodeExecutionRequests flattens execution requests for the SSZ envelope.
func encodeExecutionRequests(requests pb.ExecutionRequester) ([][]byte, error) {
	encoded, err := requests.FlattenRequests()
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode execution requests")
	}
	reqs := make([][]byte, len(encoded))
	for i := range encoded {
		reqs[i] = encoded[i]
	}
	return reqs, nil
}

// ForkchoiceUpdated updates fork choice over POST /engine/v1/forkchoice.
func (e *sszEngine) ForkchoiceUpdated(ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (*pb.PayloadIDBytes, []byte, error) {
	if attrs == nil {
		return nil, nil, errors.New("nil payload attributer")
	}
	ver, update, err := buildForkchoiceUpdate(state, attrs)
	if err != nil {
		return nil, nil, err
	}
	if err := e.rejectIfUnsupportedFork(ver); err != nil {
		return nil, nil, err
	}

	e.fcuMu.Lock()
	defer e.fcuMu.Unlock()

	observeSSZBody(methodForkchoiceUpdated, directionRequest, update)
	resp, err := e.client.ForkchoiceUpdated(ctx, ver, update)
	if err != nil {
		return nil, nil, mapEngineError(err)
	}
	observeSSZBody(methodForkchoiceUpdated, directionResponse, resp)
	return forkchoiceResult(resp)
}

// buildForkchoiceUpdate builds the fork-specific SSZ request body.
func buildForkchoiceUpdate(state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (int, ssz.Marshaler, error) {
	switch attrs.Version() {
	case version.Deneb, version.Electra, version.Fulu:
		a, err := attrs.PbV3()
		if err != nil {
			return 0, nil, err
		}
		var list []*pb.PayloadAttributesV3
		if a != nil {
			list = []*pb.PayloadAttributesV3{a}
		}
		return version.Fulu, &enginev2.ForkchoiceUpdateFulu{
			ForkchoiceState:   state,
			PayloadAttributes: list,
		}, nil
	case version.Gloas:
		a, err := attrs.PbV4()
		if err != nil {
			return 0, nil, err
		}
		var list []*pb.PayloadAttributesV4
		if a != nil {
			list = []*pb.PayloadAttributesV4{a}
		}
		return version.Gloas, &enginev2.ForkchoiceUpdateGloas{
			ForkchoiceState:   state,
			PayloadAttributes: list,
		}, nil
	default:
		return 0, nil, errors.Errorf("ssz-http engine transport: no ForkchoiceUpdate container for attribute version %s", version.String(attrs.Version()))
	}
}

// forkchoiceResult maps SSZ forkchoice status onto the JSON-RPC transport contract.
func forkchoiceResult(resp *enginev2.ForkchoiceUpdateResponse) (*pb.PayloadIDBytes, []byte, error) {
	if resp.PayloadStatus == nil {
		return nil, nil, ErrNilResponse
	}
	status := resp.PayloadStatus
	if valErr, ok := enginev2.OptionalBytes(status.ValidationError); ok && len(valErr) > 0 {
		log.WithError(errors.New(string(valErr))).Error("Got a validation error in forkChoiceUpdated")
	}
	lvh, _ := enginev2.OptionalBytes(status.LatestValidHash)
	switch status.Enum() {
	case enginev2.PayloadStatusSyncing:
		return nil, nil, ErrAcceptedSyncingPayloadStatus
	case enginev2.PayloadStatusInvalid:
		return nil, lvh, ErrInvalidPayloadStatus
	case enginev2.PayloadStatusValid:
		return optionalPayloadID(resp.PayloadId), lvh, nil
	default:
		return nil, nil, ErrUnknownPayloadStatus
	}
}

// optionalPayloadID decodes Optional[Bytes8].
func optionalPayloadID(list [][]byte) *pb.PayloadIDBytes {
	idBytes, ok := enginev2.OptionalBytes(list)
	if !ok {
		return nil
	}
	var id pb.PayloadIDBytes
	copy(id[:], idBytes)
	return &id
}

// mapEngineError maps problem+json errors to the existing engine sentinels.
func mapEngineError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *enginehttp.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Problem.Type {
	case enginehttp.ProblemInvalidForkchoice:
		return ErrInvalidForkchoiceState
	case enginehttp.ProblemInvalidAttributes:
		return ErrInvalidPayloadAttributes
	case enginehttp.ProblemUnknownPayload:
		return ErrUnknownPayload
	case enginehttp.ProblemRequestTooLarge:
		return ErrRequestTooLarge
	case enginehttp.ProblemUnsupportedFork:
		return ErrUnsupportedFork
	case enginehttp.ProblemParseError:
		return ErrParse
	case enginehttp.ProblemInvalidRequest:
		return ErrInvalidRequest
	case enginehttp.ProblemMethodNotFound:
		return ErrMethodNotFound
	case enginehttp.ProblemInvalidBody:
		return ErrInvalidParams
	case enginehttp.ProblemInternal:
		return ErrInternal
	default:
		return err
	}
}

// GetPayload retrieves a built payload over GET /engine/v1/payloads/{id}.
func (e *sszEngine) GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error) {
	ver, out, err := builtPayloadForSlot(slot)
	if err != nil {
		return nil, err
	}
	if err := e.rejectIfUnsupportedFork(ver); err != nil {
		return nil, err
	}
	if err := e.client.GetPayload(ctx, ver, payloadId, out); err != nil {
		return nil, mapEngineError(err)
	}
	if m, ok := out.(ssz.Marshaler); ok {
		observeSSZBody(methodGetPayload, directionResponse, m)
	}
	bundle, err := builtPayloadToBundle(out)
	if err != nil {
		return nil, err
	}
	res, err := blocks.NewGetPayloadResponse(bundle)
	if err != nil {
		return nil, errors.Wrap(err, "new get payload response")
	}
	return res, nil
}

// builtPayloadForSlot selects the response container for a slot.
func builtPayloadForSlot(slot primitives.Slot) (int, ssz.Unmarshaler, error) {
	epoch := slots.ToEpoch(slot)
	if epoch >= params.BeaconConfig().GloasForkEpoch {
		return version.Gloas, &enginev2.BuiltPayloadGloas{}, nil
	}
	if epoch >= params.BeaconConfig().FuluForkEpoch {
		return version.Fulu, &enginev2.BuiltPayloadFulu{}, nil
	}
	return 0, nil, errors.Errorf("ssz-http engine transport: no BuiltPayload container for slot %d (pre-Fulu)", slot)
}

// builtPayloadToBundle maps the SSZ response onto the existing proto bundle.
func builtPayloadToBundle(out ssz.Unmarshaler) (proto.Message, error) {
	switch p := out.(type) {
	case *enginev2.BuiltPayloadFulu:
		return &pb.ExecutionBundleFulu{
			Payload:               p.Payload,
			Value:                 p.BlockValue,
			BlobsBundle:           p.BlobsBundle,
			ShouldOverrideBuilder: p.ShouldOverrideBuilder,
			ExecutionRequests:     p.ExecutionRequests,
		}, nil
	case *enginev2.BuiltPayloadGloas:
		return &pb.ExecutionBundleGloas{
			Payload:               p.Payload,
			Value:                 p.BlockValue,
			BlobsBundle:           p.BlobsBundle,
			ShouldOverrideBuilder: p.ShouldOverrideBuilder,
			ExecutionRequests:     p.ExecutionRequests,
		}, nil
	default:
		return nil, errors.Errorf("ssz-http engine transport: unexpected BuiltPayload type %T", out)
	}
}

// GetBlobs fetches blobs-and-proofs over POST /engine/v1/blobs/v1.
func (e *sszEngine) GetBlobs(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error) {
	if !e.supportsBlob("v1") {
		return nil, errors.Errorf("%s is not supported", GetBlobsV1)
	}
	if err := e.rejectIfOverLimit(limitBlobsMaxVersionedHashes, uint64(len(versionedHashes))); err != nil {
		return nil, err
	}
	req := blobsRequest(versionedHashes)
	observeSSZBody(methodGetBlobs, directionRequest, req)
	resp := &enginev2.BlobsV1Response{}
	if err := e.client.GetBlobs(ctx, 1, req, resp); err != nil {
		if errors.Is(err, enginehttp.ErrNoContent) {
			return nil, nil
		}
		return nil, mapEngineError(err)
	}
	observeSSZBody(methodGetBlobs, directionResponse, resp)
	result := make([]*pb.BlobAndProof, len(versionedHashes))
	for i := range result {
		if i >= len(resp.Entries) {
			break
		}
		entry := resp.Entries[i]
		if entry == nil || !entry.Available || entry.Contents == nil {
			continue
		}
		result[i] = &pb.BlobAndProof{Blob: entry.Contents.Blob, KzgProof: entry.Contents.Proof}
	}
	return result, nil
}

// GetBlobsV2 fetches blobs-and-cell-proofs over POST /engine/v1/blobs/v2.
func (e *sszEngine) GetBlobsV2(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error) {
	if !e.supportsBlob("v2") {
		return nil, errors.Errorf("%s is not supported", GetBlobsV2)
	}
	if flags.Get().DisableGetBlobsV2 {
		return []*pb.BlobAndProofV2{}, nil
	}
	if err := e.rejectIfOverLimit(limitBlobsMaxVersionedHashes, uint64(len(versionedHashes))); err != nil {
		return nil, err
	}
	req := blobsRequest(versionedHashes)
	observeSSZBody(methodGetBlobsV2, directionRequest, req)
	resp := &enginev2.BlobsV2Response{}
	if err := e.client.GetBlobs(ctx, 2, req, resp); err != nil {
		if errors.Is(err, enginehttp.ErrNoContent) {
			return nil, nil
		}
		return nil, mapEngineError(err)
	}
	observeSSZBody(methodGetBlobsV2, directionResponse, resp)
	result := make([]*pb.BlobAndProofV2, len(versionedHashes))
	for i := range result {
		if i >= len(resp.Entries) {
			break
		}
		entry := resp.Entries[i]
		if entry == nil || !entry.Available || entry.Contents == nil {
			continue
		}
		result[i] = &pb.BlobAndProofV2{Blob: entry.Contents.Blob, KzgProofs: entry.Contents.Proofs}
	}
	return result, nil
}

// GetBlobsV3 fetches blobs-and-cell-proofs over POST /engine/v1/blobs/v3.
func (e *sszEngine) GetBlobsV3(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error) {
	if !e.supportsBlob("v3") {
		return nil, errors.Errorf("%s is not supported", GetBlobsV3)
	}
	if err := e.rejectIfOverLimit(limitBlobsMaxVersionedHashes, uint64(len(versionedHashes))); err != nil {
		return nil, err
	}
	req := blobsRequest(versionedHashes)
	observeSSZBody(methodGetBlobsV3, directionRequest, req)
	resp := &enginev2.BlobsV2Response{}
	if err := e.client.GetBlobs(ctx, 3, req, resp); err != nil {
		if errors.Is(err, enginehttp.ErrNoContent) {
			return nil, nil
		}
		return nil, mapEngineError(err)
	}
	observeSSZBody(methodGetBlobsV3, directionResponse, resp)
	result := make([]*pb.BlobAndProofV2, len(versionedHashes))
	for i := range result {
		if i >= len(resp.Entries) {
			break
		}
		entry := resp.Entries[i]
		if entry == nil || !entry.Available || entry.Contents == nil {
			continue
		}
		result[i] = &pb.BlobAndProofV2{Blob: entry.Contents.Blob, KzgProofs: entry.Contents.Proofs}
	}
	return result, nil
}

// blobsRequest builds the blob-pool request body.
func blobsRequest(versionedHashes []common.Hash) *enginev2.BlobsRequest {
	req := &enginev2.BlobsRequest{VersionedHashes: make([][]byte, len(versionedHashes))}
	for i := range versionedHashes {
		req.VersionedHashes[i] = versionedHashes[i][:]
	}
	return req
}

// supportsBlob checks the advertised /blobs/vN revisions.
func (e *sszEngine) supportsBlob(version string) bool {
	e.capsLock.RLock()
	defer e.capsLock.RUnlock()
	if e.caps == nil {
		return true
	}
	return slices.Contains(e.caps.IndependentlyVersioned["blobs"], version)
}

// rejectIfUnsupportedFork checks supported_forks for fork-scoped endpoints.
func (e *sszEngine) rejectIfUnsupportedFork(v int) error {
	fork, err := version.ELForkName(v)
	if err != nil {
		return errors.Wrap(err, "failed to get EL fork name")
	}

	e.capsLock.RLock()
	defer e.capsLock.RUnlock()
	if e.caps == nil {
		return nil
	}
	if slices.Contains(e.caps.SupportedForks, fork) {
		return nil
	}
	return errors.Wrapf(ErrUnsupportedFork, "execution fork %q not advertised by EL capabilities", fork)
}

// ExchangeCapabilities refreshes GET /engine/v1/capabilities.
func (e *sszEngine) ExchangeCapabilities(ctx context.Context) error {
	e.capsLock.Lock()
	defer e.capsLock.Unlock()

	caps, err := e.client.Capabilities(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to exchange capabilities with execution client")
	}

	e.caps = caps
	return nil
}

// GetClientVersionV1 fetches GET /engine/v1/identity.
func (e *sszEngine) GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	return e.client.Identity(ctx)
}

// GetPayloadBodiesByHash fetches bodies over POST /engine/v1/bodies/hash.
func (e *sszEngine) GetPayloadBodiesByHash(ctx context.Context, v int, hashes []common.Hash) ([]interfaces.ExecutionPayloadBody, error) {
	if err := e.rejectIfUnsupportedFork(v); err != nil {
		return nil, err
	}
	n := uint64(len(hashes))
	maxCount, capped := e.limit(limitBodiesMaxCount)
	if !capped || n <= maxCount {
		return e.bodiesByHash(ctx, v, hashes)
	}
	result := make([]interfaces.ExecutionPayloadBody, 0, len(hashes))
	for start := uint64(0); start < n; start += maxCount {
		part, err := e.bodiesByHash(ctx, v, hashes[start:min(start+maxCount, n)])
		if err != nil {
			return nil, err
		}
		result = append(result, part...)
	}
	return result, nil
}

// bodiesByHash performs one /bodies/hash request.
func (e *sszEngine) bodiesByHash(ctx context.Context, v int, hashes []common.Hash) ([]interfaces.ExecutionPayloadBody, error) {
	out, err := newBodiesResponse(v)
	if err != nil {
		return nil, err
	}
	req := &enginev2.BodiesByHashRequest{BlockHashes: make([][]byte, len(hashes))}
	for i := range hashes {
		req.BlockHashes[i] = hashes[i][:]
	}
	observeSSZBody(methodGetPayloadBodiesByHash, directionRequest, req)
	if err := e.client.GetPayloadBodiesByHash(ctx, v, req, out); err != nil {
		return nil, mapEngineError(err)
	}
	if m, ok := out.(ssz.Marshaler); ok {
		observeSSZBody(methodGetPayloadBodiesByHash, directionResponse, m)
	}
	return bodiesEntries(out)
}

// GetPayloadBodiesByRange fetches bodies over GET /engine/v1/bodies.
func (e *sszEngine) GetPayloadBodiesByRange(ctx context.Context, v int, from, count uint64) ([]interfaces.ExecutionPayloadBody, error) {
	if err := e.rejectIfUnsupportedFork(v); err != nil {
		return nil, err
	}
	maxCount, capped := e.limit(limitBodiesMaxCount)
	if !capped || count <= maxCount {
		return e.bodiesByRange(ctx, v, from, count)
	}
	result := make([]interfaces.ExecutionPayloadBody, 0, count)
	for off := uint64(0); off < count; off += maxCount {
		part, err := e.bodiesByRange(ctx, v, from+off, min(maxCount, count-off))
		if err != nil {
			return nil, err
		}
		result = append(result, part...)
	}
	return result, nil
}

// bodiesByRange performs one /bodies range request.
func (e *sszEngine) bodiesByRange(ctx context.Context, v int, from, count uint64) ([]interfaces.ExecutionPayloadBody, error) {
	out, err := newBodiesResponse(v)
	if err != nil {
		return nil, err
	}
	if err := e.client.GetPayloadBodiesByRange(ctx, v, from, count, out); err != nil {
		return nil, mapEngineError(err)
	}
	if m, ok := out.(ssz.Marshaler); ok {
		observeSSZBody(methodGetPayloadBodiesByRange, directionResponse, m)
	}
	return bodiesEntries(out)
}

// newBodiesResponse returns an empty fork-scoped BodiesResponse.
func newBodiesResponse(v int) (ssz.Unmarshaler, error) {
	switch v {
	case version.Fulu:
		return &enginev2.BodiesResponseFulu{}, nil
	case version.Gloas:
		return &enginev2.BodiesResponseGloas{}, nil
	default:
		return nil, errors.Errorf("ssz-http engine transport: no bodies container for version %s", version.String(v))
	}
}

// bodiesEntries converts a BodiesResponse into execution payload bodies.
func bodiesEntries(out ssz.Unmarshaler) ([]interfaces.ExecutionPayloadBody, error) {
	switch resp := out.(type) {
	case *enginev2.BodiesResponseFulu:
		bodies := make([]interfaces.ExecutionPayloadBody, len(resp.Entries))
		for i, entry := range resp.Entries {
			if entry == nil || !entry.Available || entry.Body == nil {
				continue
			}
			b, err := blocks.WrappedExecutionPayloadBodyFulu(entry.Body)
			if err != nil {
				return nil, err
			}
			bodies[i] = b
		}
		return bodies, nil
	case *enginev2.BodiesResponseGloas:
		bodies := make([]interfaces.ExecutionPayloadBody, len(resp.Entries))
		for i, entry := range resp.Entries {
			if entry == nil || !entry.Available || entry.Body == nil {
				continue
			}
			b, err := blocks.WrappedExecutionPayloadBodyGloas(entry.Body)
			if err != nil {
				return nil, err
			}
			bodies[i] = b
		}
		return bodies, nil
	default:
		return nil, errors.Errorf("ssz-http engine transport: unexpected BodiesResponse type %T", out)
	}
}
