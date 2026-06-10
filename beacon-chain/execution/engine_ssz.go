package execution

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution/enginehttp"
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
	caps   *enginehttp.Capabilities // captured by the connection-setup probe.
}

func sszNotImplemented(op string) error {
	return errors.Errorf("ssz-http engine transport: %s not implemented", op)
}

// NewPayload submits a payload over POST /engine/v2/{fork}/payloads
// (replaces engine_newPayloadV*). It folds parent_beacon_block_root and
// execution_requests into the SSZ envelope; versionedHashes is dropped (the EL
// recomputes it from payload.transactions). The result maps onto the same
// (latestValidHash, sentinel) contract as jsonEngine.NewPayload.
func (e *sszEngine) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests *pb.ExecutionRequests) ([]byte, error) {
	var (
		fork       string
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

		fork = enginehttp.ForkOsaka
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

		fork = enginehttp.ForkAmsterdam
	default:
		// Currently only support from Fulu (Osaka).
		// Note that Fulu has same payload shape as Deneb.
		return nil, errors.Errorf("ssz-http engine transport: no v2 ExecutionPayloadEnvelope container for payload type %T", p)
	}

	status, err := e.client.NewPayload(ctx, fork, envelope)
	if err != nil {
		return nil, err
	}
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
	fork, update, err := buildForkchoiceUpdate(state, attrs)
	if err != nil {
		return nil, nil, err
	}
	resp, err := e.client.ForkchoiceUpdated(ctx, fork, update)
	if err != nil {
		return nil, nil, mapEngineError(err)
	}
	return forkchoiceResult(resp)
}

// buildForkchoiceUpdate selects the EL fork URL and builds the fork's
// ForkchoiceUpdate container. payload_attributes is an Optional (List[T,1]):
// absent (empty list) for a pure head update, present when a build is wanted —
// PbV3/PbV4 return nil for empty attributes, mirroring the JSON-RPC null param.
func buildForkchoiceUpdate(state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (string, ssz.Marshaler, error) {
	switch attrs.Version() {
	case version.Fulu:
		a, err := attrs.PbV3()
		if err != nil {
			return "", nil, err
		}
		var list []*pb.PayloadAttributesV3
		if a != nil {
			list = []*pb.PayloadAttributesV3{a}
		}
		return enginehttp.ForkOsaka, &enginev2.ForkchoiceUpdateFulu{
			ForkchoiceState:   state,
			PayloadAttributes: list,
		}, nil
	case version.Gloas:
		a, err := attrs.PbV4()
		if err != nil {
			return "", nil, err
		}
		var list []*pb.PayloadAttributesV4
		if a != nil {
			list = []*pb.PayloadAttributesV4{a}
		}
		// custody_columns is left absent: Prysm's forkchoice path does not yet
		// manage the EL custody set, matching the JSON-RPC V4 path. An omitted
		// field leaves the EL's custody set unchanged (execution-apis#793).
		return enginehttp.ForkAmsterdam, &enginev2.ForkchoiceUpdateGloas{
			ForkchoiceState:   state,
			PayloadAttributes: list,
		}, nil
	default:
		return "", nil, errors.Errorf("ssz-http engine transport: no v2 ForkchoiceUpdate container for attribute version %s", version.String(attrs.Version()))
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
	fork, out, err := builtPayloadForSlot(slot)
	if err != nil {
		return nil, err
	}
	if err := e.client.GetPayload(ctx, fork, payloadId, out); err != nil {
		return nil, mapEngineError(err)
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
func builtPayloadForSlot(slot primitives.Slot) (string, ssz.Unmarshaler, error) {
	epoch := slots.ToEpoch(slot)
	if epoch >= params.BeaconConfig().GloasForkEpoch {
		return enginehttp.ForkAmsterdam, &enginev2.BuiltPayloadGloas{}, nil
	}
	if epoch >= params.BeaconConfig().FuluForkEpoch {
		return enginehttp.ForkOsaka, &enginev2.BuiltPayloadFulu{}, nil
	}
	return "", nil, errors.Errorf("ssz-http engine transport: no v2 BuiltPayload container for slot %d (pre-Fulu)", slot)
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

func (e *sszEngine) GetBlobs(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error) {
	return nil, sszNotImplemented("GetBlobs")
}

func (e *sszEngine) GetBlobsV2(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error) {
	return nil, sszNotImplemented("GetBlobsV2")
}

// ExchangeCapabilities returns the supported_forks captured by the setup probe
// (no second round-trip). TODO(ssz-over-http): translate the v2 capability
// shape (independently_versioned, fork_scoped_endpoints) into the engine
// method-name semantics the capabilityCache expects.
func (e *sszEngine) ExchangeCapabilities(ctx context.Context) ([]string, error) {
	if e.caps == nil {
		return nil, nil
	}
	return e.caps.SupportedForks, nil
}

// GetClientVersionV1 fetches the EL client versions over GET /engine/v2/identity
// (replaces engine_getClientVersionV1). Prysm's own version travels in the
// X-Engine-Client-Version header on every request, so there is no handshake.
func (e *sszEngine) GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	return e.client.Identity(ctx)
}
