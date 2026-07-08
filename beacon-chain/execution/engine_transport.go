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

// engineTransport abstracts the wire transport for the engine namespace.
type engineTransport interface {
	NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests pb.ExecutionRequester) ([]byte, error)
	ForkchoiceUpdated(ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer) (*pb.PayloadIDBytes, []byte, error)
	GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error)
	GetBlobs(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error)
	GetBlobsV2(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error)
	GetBlobsV3(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProofV2, error)
	HasBlobs(ctx context.Context, versionedHashes []common.Hash) ([]bool, error)
	ExchangeCapabilities(ctx context.Context) error
	GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error)
	GetPayloadBodiesByHash(ctx context.Context, v int, hashes []common.Hash) ([]interfaces.ExecutionPayloadBody, error)
	GetPayloadBodiesByRange(ctx context.Context, v int, from, count uint64) ([]interfaces.ExecutionPayloadBody, error)

	// Supports checks if the engine transport supports a specific method.
	Supports(method string) bool
}

// engine returns the selected engine transport, falling back to JSON-RPC.
func (s *Service) engine() engineTransport {
	if s.engineTransport == nil {
		s.engineTransport = s.jsonRPCTransport()
	}
	return s.engineTransport
}

// jsonRPCTransport builds a new jsonTransport with the current RPC client.
func (s *Service) jsonRPCTransport() *jsonTransport {
	return &jsonTransport{rpc: s.rpcClient, caps: &capabilityCache{}}
}

// Delegation: Make the Execution Service can call Engine API without calling engine method.

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

func (s *Service) HasBlobs(ctx context.Context, versionedHashes []common.Hash) ([]bool, error) {
	return s.engine().HasBlobs(ctx, versionedHashes)
}

func (s *Service) ExchangeCapabilities(ctx context.Context) error {
	return s.engine().ExchangeCapabilities(ctx)
}

func (s *Service) GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	return s.engine().GetClientVersionV1(ctx)
}
