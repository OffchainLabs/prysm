package execution

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution/enginehttp"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
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

func (e *sszEngine) ForkchoiceUpdated(ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (*pb.PayloadIDBytes, []byte, error) {
	return nil, nil, sszNotImplemented("ForkchoiceUpdated")
}

func (e *sszEngine) GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error) {
	return nil, sszNotImplemented("GetPayload")
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
