package beacon

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/pagination"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filters"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	"github.com/prysmaticlabs/prysm/v5/cmd"
	consensusblocks "github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// blockContainer represents an instance of
// block along with its relevant metadata.
type blockContainer struct {
	blk         interfaces.ReadOnlySignedBeaconBlock
	root        [32]byte
	isCanonical bool
}

// ListBeaconBlocks retrieves blocks by root, slot, or epoch.
//
// The server may return multiple blocks in the case that a slot or epoch is
// provided as the filter criteria. The server may return an empty list when
// no blocks in their database match the filter criteria. This RPC should
// not return NOT_FOUND. Only one filter criteria should be used.
func (bs *Server) ListBeaconBlocks(
	ctx context.Context, req *ethpb.ListBlocksRequest,
) (*ethpb.ListBeaconBlocksResponse, error) {
	ctrs, numBlks, nextPageToken, err := bs.listBlocks(ctx, req)
	if err != nil {
		return nil, err
	}
	altCtrs, err := convertFromV1Containers(ctrs)
	if err != nil {
		return nil, err
	}
	return &ethpb.ListBeaconBlocksResponse{
		BlockContainers: altCtrs,
		TotalSize:       int32(numBlks),
		NextPageToken:   nextPageToken,
	}, nil
}

func (bs *Server) listBlocks(ctx context.Context, req *ethpb.ListBlocksRequest) ([]blockContainer, int, string, error) {
	if int(req.PageSize) > cmd.Get().MaxRPCPageSize {
		return nil, 0, "", status.Errorf(codes.InvalidArgument, "Requested page size %d can not be greater than max size %d",
			req.PageSize, cmd.Get().MaxRPCPageSize)
	}

	switch q := req.QueryFilter.(type) {
	case *ethpb.ListBlocksRequest_Epoch:
		return bs.listBlocksForEpoch(ctx, req, q)
	case *ethpb.ListBlocksRequest_Root:
		return bs.listBlocksForRoot(ctx, req, q)
	case *ethpb.ListBlocksRequest_Slot:
		return bs.listBlocksForSlot(ctx, req, q)
	case *ethpb.ListBlocksRequest_Genesis:
		return bs.listBlocksForGenesis(ctx, req, q)
	default:
		return nil, 0, "", status.Errorf(codes.InvalidArgument, "Must specify a filter criteria for fetching blocks. Criteria %T not supported", q)
	}
}

func convertFromV1Containers(ctrs []blockContainer) ([]*ethpb.BeaconBlockContainer, error) {
	protoCtrs := make([]*ethpb.BeaconBlockContainer, len(ctrs))
	var err error
	for i, c := range ctrs {
		protoCtrs[i], err = convertToBlockContainer(c.blk, c.root, c.isCanonical)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not get block container: %v", err)
		}
	}
	return protoCtrs, nil
}

func convertToBlockContainer(blk interfaces.ReadOnlySignedBeaconBlock, root [32]byte, isCanonical bool) (*ethpb.BeaconBlockContainer, error) {
	ctr := &ethpb.BeaconBlockContainer{
		BlockRoot: root[:],
		Canonical: isCanonical,
	}

	pb, err := blk.Proto()
	if err != nil {
		return nil, err
	}

	switch pbStruct := pb.(type) {
	case *ethpb.SignedBeaconBlock:
		ctr.Block = &ethpb.BeaconBlockContainer_Phase0Block{Phase0Block: pbStruct}
	case *ethpb.SignedBeaconBlockAltair:
		ctr.Block = &ethpb.BeaconBlockContainer_AltairBlock{AltairBlock: pbStruct}
	case *ethpb.SignedBlindedBeaconBlockBellatrix:
		ctr.Block = &ethpb.BeaconBlockContainer_BlindedBellatrixBlock{BlindedBellatrixBlock: pbStruct}
	case *ethpb.SignedBeaconBlockBellatrix:
		ctr.Block = &ethpb.BeaconBlockContainer_BellatrixBlock{BellatrixBlock: pbStruct}
	case *ethpb.SignedBlindedBeaconBlockCapella:
		ctr.Block = &ethpb.BeaconBlockContainer_BlindedCapellaBlock{BlindedCapellaBlock: pbStruct}
	case *ethpb.SignedBeaconBlockCapella:
		ctr.Block = &ethpb.BeaconBlockContainer_CapellaBlock{CapellaBlock: pbStruct}
	case *ethpb.SignedBlindedBeaconBlockDeneb:
		ctr.Block = &ethpb.BeaconBlockContainer_BlindedDenebBlock{BlindedDenebBlock: pbStruct}
	case *ethpb.SignedBeaconBlockDeneb:
		ctr.Block = &ethpb.BeaconBlockContainer_DenebBlock{DenebBlock: pbStruct}
	case *ethpb.SignedBlindedBeaconBlockElectra:
		ctr.Block = &ethpb.BeaconBlockContainer_BlindedElectraBlock{BlindedElectraBlock: pbStruct}
	case *ethpb.SignedBeaconBlockElectra:
		ctr.Block = &ethpb.BeaconBlockContainer_ElectraBlock{ElectraBlock: pbStruct}
	case *ethpb.SignedBlindedBeaconBlockFulu:
		ctr.Block = &ethpb.BeaconBlockContainer_BlindedFuluBlock{BlindedFuluBlock: pbStruct}
	case *ethpb.SignedBeaconBlockFulu:
		ctr.Block = &ethpb.BeaconBlockContainer_FuluBlock{FuluBlock: pbStruct}
	default:
		return nil, errors.Errorf("block type is not recognized: %d", blk.Version())
	}

	return ctr, nil
}

// listBlocksForEpoch retrieves all blocks for the provided epoch.
func (bs *Server) listBlocksForEpoch(ctx context.Context, req *ethpb.ListBlocksRequest, q *ethpb.ListBlocksRequest_Epoch) ([]blockContainer, int, string, error) {
	blks, _, err := bs.BeaconDB.Blocks(ctx, filters.NewFilter().SetStartEpoch(q.Epoch).SetEndEpoch(q.Epoch))
	if err != nil {
		return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not get blocks: %v", err)
	}

	numBlks := len(blks)
	if len(blks) == 0 {
		return []blockContainer{}, numBlks, strconv.Itoa(0), nil
	}

	start, end, nextPageToken, err := pagination.StartAndEndPage(req.PageToken, int(req.PageSize), numBlks)
	if err != nil {
		return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not paginate blocks: %v", err)
	}

	returnedBlks := blks[start:end]
	containers := make([]blockContainer, len(returnedBlks))
	for i, b := range returnedBlks {
		root, err := b.Block().HashTreeRoot()
		if err != nil {
			return nil, 0, strconv.Itoa(0), err
		}
		canonical, err := bs.CanonicalFetcher.IsCanonical(ctx, root)
		if err != nil {
			return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not determine if block is canonical: %v", err)
		}
		containers[i] = blockContainer{
			blk:         b,
			root:        root,
			isCanonical: canonical,
		}
	}

	return containers, numBlks, nextPageToken, nil
}

// listBlocksForRoot retrieves the block for the provided root.
func (bs *Server) listBlocksForRoot(ctx context.Context, _ *ethpb.ListBlocksRequest, q *ethpb.ListBlocksRequest_Root) ([]blockContainer, int, string, error) {
	blk, err := bs.BeaconDB.Block(ctx, bytesutil.ToBytes32(q.Root))
	if err != nil {
		return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not retrieve block: %v", err)
	}
	if blk == nil || blk.IsNil() {
		return []blockContainer{}, 0, strconv.Itoa(0), nil
	}
	root, err := blk.Block().HashTreeRoot()
	if err != nil {
		return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not determine block root: %v", err)
	}
	canonical, err := bs.CanonicalFetcher.IsCanonical(ctx, root)
	if err != nil {
		return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not determine if block is canonical: %v", err)
	}
	return []blockContainer{{
		blk:         blk,
		root:        root,
		isCanonical: canonical,
	}}, 1, strconv.Itoa(0), nil
}

// listBlocksForSlot retrieves all blocks for the provided slot.
func (bs *Server) listBlocksForSlot(ctx context.Context, req *ethpb.ListBlocksRequest, q *ethpb.ListBlocksRequest_Slot) ([]blockContainer, int, string, error) {
	blks, err := bs.BeaconDB.BlocksBySlot(ctx, q.Slot)
	if err != nil {
		return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not retrieve blocks for slot %d: %v", q.Slot, err)
	}
	if len(blks) == 0 {
		return []blockContainer{}, 0, strconv.Itoa(0), nil
	}

	numBlks := len(blks)

	start, end, nextPageToken, err := pagination.StartAndEndPage(req.PageToken, int(req.PageSize), numBlks)
	if err != nil {
		return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not paginate blocks: %v", err)
	}

	returnedBlks := blks[start:end]
	containers := make([]blockContainer, len(returnedBlks))
	for i, b := range returnedBlks {
		root, err := b.Block().HashTreeRoot()
		if err != nil {
			return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not determine block root: %v", err)
		}
		canonical, err := bs.CanonicalFetcher.IsCanonical(ctx, root)
		if err != nil {
			return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not determine if block is canonical: %v", err)
		}
		containers[i] = blockContainer{
			blk:         b,
			root:        root,
			isCanonical: canonical,
		}
	}
	return containers, numBlks, nextPageToken, nil
}

// listBlocksForGenesis retrieves the genesis block.
func (bs *Server) listBlocksForGenesis(ctx context.Context, _ *ethpb.ListBlocksRequest, _ *ethpb.ListBlocksRequest_Genesis) ([]blockContainer, int, string, error) {
	genBlk, err := bs.BeaconDB.GenesisBlock(ctx)
	if err != nil {
		return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not retrieve blocks for genesis slot: %v", err)
	}
	if err := consensusblocks.BeaconBlockIsNil(genBlk); err != nil {
		return []blockContainer{}, 0, strconv.Itoa(0), status.Errorf(codes.NotFound, "Could not find genesis block: %v", err)
	}
	root, err := genBlk.Block().HashTreeRoot()
	if err != nil {
		return nil, 0, strconv.Itoa(0), status.Errorf(codes.Internal, "Could not determine block root: %v", err)
	}
	return []blockContainer{{
		blk:         genBlk,
		root:        root,
		isCanonical: true,
	}}, 1, strconv.Itoa(0), nil
}

// GetChainHead retrieves information about the head of the beacon chain from
// the view of the beacon chain node.
//
// This includes the head block slot and root as well as information about
// the most recent finalized and justified slots.
// DEPRECATED: This endpoint is superseded by the /eth/v1/beacon API endpoint
func (bs *Server) GetChainHead(ctx context.Context, _ *emptypb.Empty) (*ethpb.ChainHead, error) {
	ch, err := bs.CoreService.ChainHead(ctx)
	if err != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(err.Reason), "Could not retrieve chain head: %v", err.Err)
	}
	return ch, nil
}
