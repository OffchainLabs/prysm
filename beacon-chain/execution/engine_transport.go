package execution

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution/enginehttp"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/sirupsen/logrus"
)

// engineTransport abstracts the wire transport for the engine namespace so the
// Service can speak either JSON-RPC (engine_*, jsonEngine) or REST + SSZ
// (/engine/v2/..., sszEngine; ethereum/execution-apis#793) below the public
// EngineCaller/Reconstructor surface. Methods are Prysm-typed because the
// Prysm-type <-> wire conversion is itself transport-specific. The eth_*
// namespace (ExecutionBlockByHash, GetTerminalBlockHash, HeaderBy*, ...) is not
// part of this interface — the proposal leaves it on JSON-RPC.
type engineTransport interface {
	NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests pb.ExecutionRequester) ([]byte, error)
	ForkchoiceUpdated(ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (*pb.PayloadIDBytes, []byte, error)
	GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error)
	GetBlobs(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error)
	GetBlobsV2(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error)
	GetBlobsV3(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error)
	ExchangeCapabilities(ctx context.Context) error
	GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error)
	GetPayloadBodiesByHash(ctx context.Context, v int, hashes []common.Hash) ([]interfaces.ExecutionPayloadBody, error)
	GetPayloadBodiesByRange(ctx context.Context, v int, from, count uint64) ([]interfaces.ExecutionPayloadBody, error)
}

// Transport labels for the engine metrics (metrics.go).
const (
	transportJSON = "json-rpc"
	transportSSZ  = "ssz-http"
)

// Per-endpoint method labels for the engine metrics, shared by the latency
// decorator here and the ssz body-size instrumentation in engine_ssz.go so both
// metrics align on one label set.
const (
	methodNewPayload              = "newPayload"
	methodForkchoiceUpdated       = "forkchoiceUpdated"
	methodGetPayload              = "getPayload"
	methodGetBlobs                = "getBlobs"
	methodGetBlobsV2              = "getBlobsV2"
	methodGetBlobsV3              = "getBlobsV3"
	methodGetPayloadBodiesByHash  = "getPayloadBodiesByHash"
	methodGetPayloadBodiesByRange = "getPayloadBodiesByRange"
)

// observeEngineLatency records the wall-clock latency of one engine op under its
// endpoint/transport labels. Called via defer with time.Now() captured at the
// call site so start reflects op entry.
func observeEngineLatency(method, transport string, start time.Time) {
	engineRequestLatency.WithLabelValues(method, transport).Observe(float64(time.Since(start).Milliseconds()))
}

// instrumentedEngine wraps the selected engineTransport to time each engine op,
// labeled by transport (json-rpc vs ssz-http). engine() returns the transport
// wrapped here so every op — including the bodies reconstruction path, which
// calls the transport directly rather than through the Service dispatchers — is
// timed at one seam. Body sizes are recorded separately, at each transport's wire
// layer (observeSSZBody for ssz-http, the size round-tripper for json-rpc), since
// only those layers see the marshaled bytes. The embedded engineTransport carries
// the diagnostic ops (capabilities, identity) through unmeasured.
type instrumentedEngine struct {
	engineTransport
	kind string
}

func (m *instrumentedEngine) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests pb.ExecutionRequester) ([]byte, error) {
	defer observeEngineLatency(methodNewPayload, m.kind, time.Now())
	return m.engineTransport.NewPayload(ctx, payload, versionedHashes, parentBlockRoot, executionRequests)
}

func (m *instrumentedEngine) ForkchoiceUpdated(ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (*pb.PayloadIDBytes, []byte, error) {
	defer observeEngineLatency(methodForkchoiceUpdated, m.kind, time.Now())
	return m.engineTransport.ForkchoiceUpdated(ctx, state, attrs)
}

func (m *instrumentedEngine) GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error) {
	defer observeEngineLatency(methodGetPayload, m.kind, time.Now())
	return m.engineTransport.GetPayload(ctx, payloadId, slot)
}

func (m *instrumentedEngine) GetBlobs(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error) {
	defer observeEngineLatency(methodGetBlobs, m.kind, time.Now())
	return m.engineTransport.GetBlobs(ctx, versionedHashes)
}

func (m *instrumentedEngine) GetBlobsV2(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error) {
	defer observeEngineLatency(methodGetBlobsV2, m.kind, time.Now())
	return m.engineTransport.GetBlobsV2(ctx, versionedHashes)
}

func (m *instrumentedEngine) GetBlobsV3(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error) {
	defer observeEngineLatency(methodGetBlobs, m.kind, time.Now())
	return m.engineTransport.GetBlobsV3(ctx, versionedHashes)
}

func (m *instrumentedEngine) GetPayloadBodiesByHash(ctx context.Context, v int, hashes []common.Hash) ([]interfaces.ExecutionPayloadBody, error) {
	defer observeEngineLatency(methodGetPayloadBodiesByHash, m.kind, time.Now())
	return m.engineTransport.GetPayloadBodiesByHash(ctx, v, hashes)
}

func (m *instrumentedEngine) GetPayloadBodiesByRange(ctx context.Context, v int, from, count uint64) ([]interfaces.ExecutionPayloadBody, error) {
	defer observeEngineLatency(methodGetPayloadBodiesByRange, m.kind, time.Now())
	return m.engineTransport.GetPayloadBodiesByRange(ctx, v, from, count)
}

// engine returns the engine transport selected for the current connection,
// wrapped in instrumentedEngine for per-endpoint latency metrics. JSON-RPC is
// the default; selectEngineTransport sets sszTransport when the feature flag is
// on and the execution client serves the v2 (REST+SSZ) surface. The jsonEngine
// is built once per connection and cached so it owns a stable capability cache
// across calls (selectEngineTransport clears it on reconnect).
func (s *Service) engine() engineTransport {
	if s.sszTransport != nil {
		return &instrumentedEngine{engineTransport: s.sszTransport, kind: transportSSZ}
	}
	if s.jsonTransport == nil {
		// engineLabelingClient tags engine_* call contexts so engineSizeRoundTripper
		// can record engine_body_size_bytes{transport="json-rpc"} (comparable to the
		// ssz-http sizes); eth_* calls go through s.rpcClient directly, untagged.
		s.jsonTransport = &jsonEngine{rpc: &engineLabelingClient{RPCClient: s.rpcClient}, caps: &capabilityCache{}}
	}
	return &instrumentedEngine{engineTransport: s.jsonTransport, kind: transportJSON}
}

// selectEngineTransport decides whether to drive the engine API over
// SSZ-over-HTTP (execution-apis#793) or JSON-RPC for this connection. With the
// feature flag set it probes GET /engine/v2/capabilities; on success the SSZ
// transport is used, otherwise (no v2 surface, non-h2c endpoint, or probe
// error) it falls back to JSON-RPC for the connection's lifetime — per spec
// there is no per-method fallback ladder. Called on every (re)connection.
func (s *Service) selectEngineTransport(ctx context.Context, endpoint network.Endpoint) {
	// Reset the transport and its capability cache (repopulated on ExchangeCapabilities).
	s.sszTransport = nil
	s.jsonTransport = nil

	if !features.Get().EnableEngineSSZHTTP {
		return
	}

	client, err := enginehttp.New(enginehttp.Config{
		BaseURL:       endpoint.Url,
		JWTSecret:     []byte(endpoint.Auth.Value),
		JWTID:         endpoint.Auth.JwtId,
		ClientVersion: "Prysm/" + version.SemanticVersion(),
	})
	if err != nil {
		engineSSZHTTPFallbackCount.Inc()
		log.WithError(err).Warn("SSZ-over-HTTP engine transport unavailable; using JSON-RPC")
		return
	}

	caps, err := client.Capabilities(ctx)
	if err != nil {
		engineSSZHTTPFallbackCount.Inc()
		log.WithError(err).Info("Execution client has no engine v2 (REST+SSZ) surface; using JSON-RPC")
		return
	}

	s.sszTransport = &sszEngine{client: client, caps: caps}

	log.WithFields(logrus.Fields{
		"supportedForks":         caps.SupportedForks,
		"forkScopedEndpoints":    caps.ForkScopedEndpoints,
		"independentlyVersioned": caps.IndependentlyVersioned,
		"unscopedEndpoints":      caps.UnscopedEndpoints,
	}).Info("Using SSZ-over-HTTP engine transport")
}

// Public EngineCaller-facing entry points. Each dispatches to the selected
// transport so callers (block import, fork choice, proposer, reconstruction)
// are unaware of which wire transport is in use.

func (s *Service) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests pb.ExecutionRequester) ([]byte, error) {
	return s.engine().NewPayload(ctx, payload, versionedHashes, parentBlockRoot, executionRequests)
}

func (s *Service) ForkchoiceUpdated(ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (*pb.PayloadIDBytes, []byte, error) {
	return s.engine().ForkchoiceUpdated(ctx, state, attrs)
}

func (s *Service) GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error) {
	return s.engine().GetPayload(ctx, payloadId, slot)
}

func (s *Service) GetBlobs(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error) {
	return s.engine().GetBlobs(ctx, versionedHashes)
}

func (s *Service) GetBlobsV2(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error) {
	return s.engine().GetBlobsV2(ctx, versionedHashes)
}

func (s *Service) GetBlobsV3(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error) {
	return s.engine().GetBlobsV3(ctx, versionedHashes)
}

func (s *Service) ExchangeCapabilities(ctx context.Context) error {
	return s.engine().ExchangeCapabilities(ctx)
}

func (s *Service) GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	return s.engine().GetClientVersionV1(ctx)
}
