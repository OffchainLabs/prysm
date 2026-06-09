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
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
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

func (e *sszEngine) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests *pb.ExecutionRequests) ([]byte, error) {
	return nil, sszNotImplemented("NewPayload")
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

func (e *sszEngine) GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	return nil, sszNotImplemented("GetClientVersionV1")
}
