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

// engineTransport abstracts the wire transport for the engine namespace.
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

// Per-endpoint method labels for engine metrics.
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

// observeEngineLatency records one engine operation latency.
func observeEngineLatency(method, transport string, start time.Time) {
	engineRequestLatency.WithLabelValues(method, transport).Observe(float64(time.Since(start).Milliseconds()))
}

// instrumentedEngine times engine operations by transport.
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
	defer observeEngineLatency(methodGetBlobsV3, m.kind, time.Now())
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

// engine returns the selected engine transport for the current connection.
func (s *Service) engine() engineTransport {
	if s.sszTransport != nil {
		return &instrumentedEngine{engineTransport: s.sszTransport, kind: transportSSZ}
	}
	if s.jsonTransport == nil {
		s.jsonTransport = &jsonEngine{rpc: &engineLabelingClient{RPCClient: s.rpcClient}, caps: &capabilityCache{}}
	}
	return &instrumentedEngine{engineTransport: s.jsonTransport, kind: transportJSON}
}

// selectEngineTransport probes REST + SSZ support for this connection.
func (s *Service) selectEngineTransport(ctx context.Context, endpoint network.Endpoint) {
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
		log.WithError(err).Info("Execution client has no REST+SSZ engine surface; using JSON-RPC")
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
