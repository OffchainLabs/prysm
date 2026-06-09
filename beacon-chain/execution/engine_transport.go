package execution

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/ethereum/go-ethereum/common"
)

// engineTransport abstracts the wire transport for the engine namespace so the
// Service can speak either JSON-RPC (engine_*, jsonEngine) or — in a later
// change — REST + SSZ (ethereum/execution-apis#793) below the public
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
	ExchangeCapabilities(ctx context.Context) ([]string, error)
	GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error)
}

// engine returns the engine transport for the current connection. Today this is
// always the JSON-RPC implementation; an alternative SSZ-over-HTTP transport is
// selected in a later change.
func (s *Service) engine() engineTransport {
	return jsonEngine{rpc: s.rpcClient, caps: s.capabilityCache}
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

func (s *Service) ExchangeCapabilities(ctx context.Context) ([]string, error) {
	return s.engine().ExchangeCapabilities(ctx)
}

func (s *Service) GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	return s.engine().GetClientVersionV1(ctx)
}
