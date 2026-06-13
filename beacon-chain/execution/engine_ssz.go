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

// sszEngine drives the engine namespace over the REST + SSZ Engine API v2
// (ethereum/execution-apis#793) via enginehttp.Client. It implements
// engineTransport. Only Capabilities is wired so far; the remaining operations
// land one group per PR (Phase 4) and until then return sszNotImplemented.
type sszEngine struct {
	client *enginehttp.Client

	capsLock sync.RWMutex
	caps     *enginehttp.Capabilities

	// fcuMu serializes POST /forkchoice on this connection: the spec allows only
	// one forkchoice request in flight per connection and the CL MUST await the
	// response before issuing the next, and MUST NOT rely on the EL to reorder
	// dependent requests.
	fcuMu sync.Mutex
}

func sszNotImplemented(op string) error {
	return errors.Errorf("ssz-http engine transport: %s not implemented", op)
}

// Body-size directions for the engine_body_size_bytes metric (metrics.go).
const (
	directionRequest  = "request"
	directionResponse = "response"
)

// observeSSZBody records the SSZ wire size of one request/response body under the
// endpoint and direction labels; the transport is always ssz-http (the JSON-RPC
// client does not expose wire sizes). m must be non-nil — a decoded container's
// SizeSSZ equals its wire byte length.
func observeSSZBody(method, direction string, m ssz.Marshaler) {
	engineBodySize.WithLabelValues(method, transportSSZ, direction).Observe(float64(m.SizeSSZ()))
}

// EL-advertised per-request limits, keys of the GET /engine/v2/capabilities
// "limits" map (execution-apis#793). They are upper bounds the CL must respect;
// exceeding one earns a 413 request-too-large from the EL. bodies requests are
// chunked to stay within the cap (functionality-preserving); blobs and payload
// requests are atomic and so are rejected client-side with the same
// ErrRequestTooLarge sentinel the 413 maps to.
const (
	limitBodiesMaxCount          = "bodies.max_count"
	limitBlobsMaxVersionedHashes = "blobs.max_versioned_hashes"
	limitPayloadMaxBytes         = "payload.max_bytes"
)

// limit returns the EL-advertised limits.* value for key and whether it imposes
// a client-side cap. A nil capability document or an absent/zero value means no
// cap here (the EL's own limits still apply), mirroring supportsBlob's
// defensive default.
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

// rejectIfOverLimit returns ErrRequestTooLarge (the sentinel the EL's 413
// request-too-large maps to) when n exceeds the advertised cap for key. Used for
// atomic requests that cannot be split — blobs (all-or-nothing) and a single
// payload envelope.
func (e *sszEngine) rejectIfOverLimit(key string, n uint64) error {
	if maxN, ok := e.limit(key); ok && n > maxN {
		return errors.Wrapf(ErrRequestTooLarge, "request of %d exceeds advertised %s=%d", n, key, maxN)
	}
	return nil
}

// NewPayload submits a payload over POST /engine/v2/{fork}/payloads
// (replaces engine_newPayloadV*). It folds parent_beacon_block_root and
// execution_requests into the SSZ envelope; versionedHashes is dropped (the EL
// recomputes it from payload.transactions). The result maps onto the same
// (latestValidHash, sentinel) contract as jsonEngine.NewPayload.
func (e *sszEngine) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests *pb.ExecutionRequests) ([]byte, error) {
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
		// Currently only support from Fulu (Osaka).
		// Note that Fulu has same payload shape as Deneb.
		return nil, errors.Errorf("ssz-http engine transport: no v2 ExecutionPayloadEnvelope container for payload type %T", p)
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

// payloadStatusResult maps a v2 PayloadStatus onto the (latestValidHash, error)
// contract EngineCaller consumers expect, identical to jsonEngine.NewPayload.
// The removed INVALID_BLOCK_HASH folds into INVALID; ACCEPTED/SYNCING, INVALID
// and VALID map to the same sentinels as JSON-RPC.
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

// encodeExecutionRequests flattens execution requests into the SSZ envelope's
// List[ByteList, MAX_REQUESTS] field, matching the JSON-RPC flattening.
func encodeExecutionRequests(requests *pb.ExecutionRequests) ([][]byte, error) {
	encoded, err := pb.EncodeExecutionRequests(requests)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode execution requests")
	}
	reqs := make([][]byte, len(encoded))
	for i := range encoded {
		reqs[i] = encoded[i]
	}
	return reqs, nil
}

// ForkchoiceUpdated updates fork choice (and optionally starts a build) over
// POST /engine/v2/{fork}/forkchoice (replaces engine_forkchoiceUpdatedV*). The
// returned payload_id is the EL's opaque token, echoed verbatim. Results and
// errors map onto the same returns/sentinels as jsonEngine.ForkchoiceUpdated.
func (e *sszEngine) ForkchoiceUpdated(ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (*pb.PayloadIDBytes, []byte, error) {
	if attrs == nil {
		return nil, nil, errors.New("nil payload attributer")
	}
	ver, update, err := buildForkchoiceUpdate(state, attrs)
	if err != nil {
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

// buildForkchoiceUpdate selects the EL fork URL and builds the fork's
// ForkchoiceUpdate container. payload_attributes is an Optional (List[T,1]):
// absent (empty list) for a pure head update, present when a build is wanted —
// PbV3/PbV4 return nil for empty attributes, mirroring the JSON-RPC null param.
func buildForkchoiceUpdate(state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (int, ssz.Marshaler, error) {
	switch attrs.Version() {
	case version.Fulu:
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
		// custody_columns is left absent: Prysm's forkchoice path does not yet
		// manage the EL custody set, matching the JSON-RPC V4 path. An omitted
		// field leaves the EL's custody set unchanged (execution-apis#793).
		return version.Gloas, &enginev2.ForkchoiceUpdateGloas{
			ForkchoiceState:   state,
			PayloadAttributes: list,
		}, nil
	default:
		return 0, nil, errors.Errorf("ssz-http engine transport: no v2 ForkchoiceUpdate container for attribute version %s", version.String(attrs.Version()))
	}
}

// forkchoiceResult maps a v2 ForkchoiceUpdateResponse onto the same returns and
// sentinels as jsonEngine.ForkchoiceUpdated. forkchoice returns the restricted
// enum (VALID|INVALID|SYNCING); an out-of-range value is a protocol error.
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

// optionalPayloadID reads the opaque server-assigned payload_id from its
// Optional[Bytes8] list, copying the bytes verbatim (never recomputed). Absent
// (no build started) yields nil.
func optionalPayloadID(list [][]byte) *pb.PayloadIDBytes {
	idBytes, ok := enginev2.OptionalBytes(list)
	if !ok {
		return nil
	}
	var id pb.PayloadIDBytes
	copy(id[:], idBytes)
	return &id
}

// mapEngineError converts an enginehttp transport error (RFC 7807 problem+json,
// execution-apis#793) into the JSON-RPC sentinels consumers already branch on,
// keyed on the stable `type` URI. Non-*Error errors (timeouts, ErrNoContent,
// IO) pass through unchanged.
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

// GetPayload retrieves a built payload over GET /engine/v2/{fork}/payloads/{id}
// (replaces engine_getPayloadV*). The fork is selected by the slot; the opaque
// id is echoed into the path. The v2 BuiltPayload is converted into the existing
// ExecutionBundle proto and run through blocks.NewGetPayloadResponse so the
// result is identical to the JSON-RPC path. The response is never cached.
func (e *sszEngine) GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error) {
	ver, out, err := builtPayloadForSlot(slot)
	if err != nil {
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

// builtPayloadForSlot selects the EL fork URL and a fresh BuiltPayload container
// to decode into, by the slot's fork (mirrors getPayloadMethodAndMessage).
func builtPayloadForSlot(slot primitives.Slot) (int, ssz.Unmarshaler, error) {
	epoch := slots.ToEpoch(slot)
	if epoch >= params.BeaconConfig().GloasForkEpoch {
		return version.Gloas, &enginev2.BuiltPayloadGloas{}, nil
	}
	if epoch >= params.BeaconConfig().FuluForkEpoch {
		return version.Fulu, &enginev2.BuiltPayloadFulu{}, nil
	}
	return 0, nil, errors.Errorf("ssz-http engine transport: no v2 BuiltPayload container for slot %d (pre-Fulu)", slot)
}

// builtPayloadToBundle maps a decoded v2 BuiltPayload onto the existing
// ExecutionBundle proto (a flat field copy — both reuse the same v1 inner
// payload/blobs types) so blocks.NewGetPayloadResponse builds the response the
// same way it does for the JSON-RPC path.
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

// GetBlobs fetches blobs-and-proofs over POST /engine/v2/blobs/v1 (replaces
// engine_getBlobsV1). The result is request-aligned: one slot per requested
// hash, nil where the EL reported available=false. A 204 (ErrNoContent) means
// the EL cannot serve the request, returned as an empty result.
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
	entries := *resp
	result := make([]*pb.BlobAndProof, len(versionedHashes))
	for i := range result {
		if i >= len(entries) {
			break
		}
		entry := entries[i]
		if entry == nil || !entry.Available || entry.Contents == nil {
			continue
		}
		result[i] = &pb.BlobAndProof{Blob: entry.Contents.Blob, KzgProof: entry.Contents.Proof}
	}
	return result, nil
}

// GetBlobsV2 fetches blobs-and-cell-proofs over POST /engine/v2/blobs/v2
// (replaces engine_getBlobsV2, all-or-nothing). A 204 (ErrNoContent) means the
// EL cannot serve the request or at least one blob is missing — returned as an
// empty result, matching the JSON-RPC "nothing returned" path.
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
	entries := *resp
	result := make([]*pb.BlobAndProofV2, len(versionedHashes))
	for i := range result {
		if i >= len(entries) {
			break
		}
		entry := entries[i]
		if entry == nil || !entry.Available || entry.Contents == nil {
			continue
		}
		result[i] = &pb.BlobAndProofV2{Blob: entry.Contents.Blob, KzgProofs: entry.Contents.Proofs}
	}
	return result, nil
}

// blobsRequest builds the SSZ List[VersionedHash] request body shared by the
// blob-pool endpoints.
func blobsRequest(versionedHashes []common.Hash) *enginev2.BlobsRequest {
	req := make(enginev2.BlobsRequest, len(versionedHashes))
	for i := range versionedHashes {
		req[i] = versionedHashes[i][:]
	}
	return &req
}

// supportsBlob reports whether the EL advertised the given /blobs/vN revision in
// its capabilities (the SSZ-native equivalent of jsonEngine's caps.has check).
func (e *sszEngine) supportsBlob(version string) bool {
	e.capsLock.RLock()
	defer e.capsLock.RUnlock()
	if e.caps == nil {
		return true
	}
	return slices.Contains(e.caps.IndependentlyVersioned["blobs"], version)
}

// ExchangeCapabilities probes the EL's v2 capabilities over GET /engine/v2/capabilities
// and saves them in the engine's capability.
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

// GetClientVersionV1 fetches the EL client versions over GET /engine/v2/identity
// (replaces engine_getClientVersionV1). Prysm's own version travels in the
// X-Engine-Client-Version header on every request, so there is no handshake.
func (e *sszEngine) GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	return e.client.Identity(ctx)
}

// GetPayloadBodiesByHash fetches bodies over POST /engine/v2/{fork}/bodies/hash
// (replaces engine_getPayloadBodiesByHashV1). Entries are request-aligned;
// available=false maps to a nil body (the reconstructor's missing marker).
// Requests over the advertised bodies.max_count are split into in-order
// sub-batches so the concatenated result stays aligned with hashes.
func (e *sszEngine) GetPayloadBodiesByHash(ctx context.Context, v int, hashes []common.Hash) ([]interfaces.ExecutionPayloadBody, error) {
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

// bodiesByHash performs one /bodies/hash call for hashes within the EL's cap.
func (e *sszEngine) bodiesByHash(ctx context.Context, v int, hashes []common.Hash) ([]interfaces.ExecutionPayloadBody, error) {
	out, err := newBodiesResponse(v)
	if err != nil {
		return nil, err
	}
	req := make(enginev2.BodiesByHashRequest, len(hashes))
	for i := range hashes {
		req[i] = hashes[i][:]
	}
	observeSSZBody(methodGetPayloadBodiesByHash, directionRequest, &req)
	if err := e.client.GetPayloadBodiesByHash(ctx, v, &req, out); err != nil {
		return nil, mapEngineError(err)
	}
	if m, ok := out.(ssz.Marshaler); ok {
		observeSSZBody(methodGetPayloadBodiesByHash, directionResponse, m)
	}
	return bodiesEntries(out)
}

// GetPayloadBodiesByRange fetches bodies over GET /engine/v2/{fork}/bodies?from&count
// (replaces engine_getPayloadBodiesByRangeV1). A range wider than the advertised
// bodies.max_count is split into consecutive windows; the in-order concatenation
// covers [from, from+count) exactly.
func (e *sszEngine) GetPayloadBodiesByRange(ctx context.Context, v int, from, count uint64) ([]interfaces.ExecutionPayloadBody, error) {
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

// bodiesByRange performs one /bodies range call for a window within the EL's cap.
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

// newBodiesResponse returns an empty fork-scoped BodiesResponse to decode into.
// Only the Fulu (osaka) and Gloas (amsterdam) body containers exist in
// proto/engine/v2; pre-osaka forks have no v2 bodies container.
func newBodiesResponse(v int) (ssz.Unmarshaler, error) {
	switch v {
	case version.Fulu:
		return &enginev2.BodiesResponseFulu{}, nil
	case version.Gloas:
		return &enginev2.BodiesResponseGloas{}, nil
	default:
		return nil, errors.Errorf("ssz-http engine transport: no v2 bodies container for version %s", version.String(v))
	}
}

// bodiesEntries converts a fork-scoped BodiesResponse into the transport-neutral
// []interfaces.ExecutionPayloadBody the reconstructors consume.
func bodiesEntries(out ssz.Unmarshaler) ([]interfaces.ExecutionPayloadBody, error) {
	switch resp := out.(type) {
	case *enginev2.BodiesResponseFulu:
		bodies := make([]interfaces.ExecutionPayloadBody, len(*resp))
		for i, entry := range *resp {
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
		bodies := make([]interfaces.ExecutionPayloadBody, len(*resp))
		for i, entry := range *resp {
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
