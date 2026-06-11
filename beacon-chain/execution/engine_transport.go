package execution

import (
	"context"

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
	NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests *pb.ExecutionRequests) ([]byte, error)
	ForkchoiceUpdated(ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (*pb.PayloadIDBytes, []byte, error)
	GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error)
	GetBlobs(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error)
	GetBlobsV2(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error)
	ExchangeCapabilities(ctx context.Context) error
	GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error)
}

// engine returns the engine transport selected for the current connection.
// JSON-RPC is the default; selectEngineTransport sets sszTransport when the
// feature flag is on and the execution client serves the v2 (REST+SSZ) surface.
// The jsonEngine is built once per connection and cached so it owns a stable
// capability cache across calls (selectEngineTransport clears it on reconnect).
func (s *Service) engine() engineTransport {
	if s.sszTransport != nil {
		return s.sszTransport
	}
	if s.jsonTransport == nil {
		s.jsonTransport = &jsonEngine{rpc: s.rpcClient, caps: &capabilityCache{}}
	}
	return s.jsonTransport
}

// selectEngineTransport decides whether to drive the engine API over
// SSZ-over-HTTP (execution-apis#793) or JSON-RPC for this connection. With the
// feature flag set it probes GET /engine/v2/capabilities; on success the SSZ
// transport is used, otherwise (no v2 surface, non-h2c endpoint, or probe
// error) it falls back to JSON-RPC for the connection's lifetime — per spec
// there is no per-method fallback ladder. Called on every (re)connection.
func (s *Service) selectEngineTransport(ctx context.Context, endpoint network.Endpoint) {
	// Reset the transport.
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
		log.WithError(err).Warn("SSZ-over-HTTP engine transport unavailable; using JSON-RPC")
		return
	}

	caps, err := client.Capabilities(ctx)
	if err != nil {
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

func (s *Service) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests *pb.ExecutionRequests) ([]byte, error) {
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

func (s *Service) ExchangeCapabilities(ctx context.Context) error {
	return s.engine().ExchangeCapabilities(ctx)
}

func (s *Service) GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	return s.engine().GetClientVersionV1(ctx)
}
