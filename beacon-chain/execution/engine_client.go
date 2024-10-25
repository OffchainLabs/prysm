package execution

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/execution/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	payloadattribute "github.com/prysmaticlabs/prysm/v5/consensus-types/payload-attribute"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	pb "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

var (
	supportedEngineEndpoints = []string{
		NewPayloadMethod,
		NewPayloadMethodV2,
		NewPayloadMethodV3,
		NewPayloadMethodV4,
		ForkchoiceUpdatedMethod,
		ForkchoiceUpdatedMethodV2,
		ForkchoiceUpdatedMethodV3,
		GetPayloadMethod,
		GetPayloadMethodV2,
		GetPayloadMethodV3,
		GetPayloadMethodV4,
		GetPayloadBodiesByHashV1,
		GetPayloadBodiesByRangeV1,
	}
)

const (
	// NewPayloadMethod v1 request string for JSON-RPC.
	NewPayloadMethod = "engine_newPayloadV1"
	// NewPayloadMethodV2 v2 request string for JSON-RPC.
	NewPayloadMethodV2 = "engine_newPayloadV2"
	NewPayloadMethodV3 = "engine_newPayloadV3"
	// NewPayloadMethodV4 is the engine_newPayloadVX method added at Electra.
	NewPayloadMethodV4 = "engine_newPayloadV4"
	// ForkchoiceUpdatedMethod v1 request string for JSON-RPC.
	ForkchoiceUpdatedMethod = "engine_forkchoiceUpdatedV1"
	// ForkchoiceUpdatedMethodV2 v2 request string for JSON-RPC.
	ForkchoiceUpdatedMethodV2 = "engine_forkchoiceUpdatedV2"
	// ForkchoiceUpdatedMethodV3 v3 request string for JSON-RPC.
	ForkchoiceUpdatedMethodV3 = "engine_forkchoiceUpdatedV3"
	// GetPayloadMethod v1 request string for JSON-RPC.
	GetPayloadMethod = "engine_getPayloadV1"
	// GetPayloadMethodV2 v2 request string for JSON-RPC.
	GetPayloadMethodV2 = "engine_getPayloadV2"
	// GetPayloadMethodV3 is the get payload method added for deneb
	GetPayloadMethodV3 = "engine_getPayloadV3"
	// GetPayloadMethodV4 is the get payload method added for electra
	GetPayloadMethodV4 = "engine_getPayloadV4"
	// BlockByHashMethod request string for JSON-RPC.
	BlockByHashMethod = "eth_getBlockByHash"
	// BlockByNumberMethod request string for JSON-RPC.
	BlockByNumberMethod = "eth_getBlockByNumber"
	// GetPayloadBodiesByHashV1 is the engine_getPayloadBodiesByHashX JSON-RPC method for pre-Electra payloads.
	GetPayloadBodiesByHashV1 = "engine_getPayloadBodiesByHashV1"
	// GetPayloadBodiesByRangeV1 is the engine_getPayloadBodiesByRangeX JSON-RPC method for pre-Electra payloads.
	GetPayloadBodiesByRangeV1 = "engine_getPayloadBodiesByRangeV1"
	// ExchangeCapabilities request string for JSON-RPC.
	ExchangeCapabilities = "engine_exchangeCapabilities"
	// GetBlobsV1 request string for JSON-RPC.
	GetBlobsV1 = "engine_getBlobsV1"
	// Defines the seconds before timing out engine endpoints with non-block execution semantics.
	defaultEngineTimeout = time.Second
)

var errInvalidPayloadBodyResponse = errors.New("engine api payload body response is invalid")

// ForkchoiceUpdatedResponse is the response kind received by the
// engine_forkchoiceUpdatedV1 endpoint.
type ForkchoiceUpdatedResponse struct {
	Status          *pb.PayloadStatus  `json:"payloadStatus"`
	PayloadId       *pb.PayloadIDBytes `json:"payloadId"`
	ValidationError string             `json:"validationError"`
}

// Reconstructor defines a service responsible for reconstructing full beacon chain objects by utilizing the execution API and making requests through the execution client.
type Reconstructor interface {
	ReconstructFullBlock(
		ctx context.Context, blindedBlock interfaces.ReadOnlySignedBeaconBlock,
	) (interfaces.SignedBeaconBlock, error)
	ReconstructFullBellatrixBlockBatch(
		ctx context.Context, blindedBlocks []interfaces.ReadOnlySignedBeaconBlock,
	) ([]interfaces.SignedBeaconBlock, error)
	ReconstructBlobSidecars(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte, indices []bool) ([]blocks.VerifiedROBlob, error)
}

// EngineCaller defines a client that can interact with an Ethereum
// execution node's engine service via JSON-RPC.
type EngineCaller interface {
	NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests *pb.ExecutionRequests) ([]byte, error)
	ForkchoiceUpdated(
		ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer,
	) (*pb.PayloadIDBytes, []byte, error)
	GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error)
	ExecutionBlockByHash(ctx context.Context, hash common.Hash, withTxs bool) (*pb.ExecutionBlock, error)
	GetTerminalBlockHash(ctx context.Context, transitionTime uint64) ([]byte, bool, error)
}

var ErrEmptyBlockHash = errors.New("Block hash is empty 0x0000...")

// NewPayload request calls the engine_newPayloadVX method via JSON-RPC.
func (s *Service) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash, executionRequests *pb.ExecutionRequests) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.NewPayload")
	defer span.End()
	start := time.Now()
	defer func() {
		newPayloadLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()

	d := time.Now().Add(time.Duration(params.BeaconConfig().ExecutionEngineTimeoutValue) * time.Second)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()
	result := &pb.PayloadStatus{}

	switch payload.Proto().(type) {
	case *pb.ExecutionPayload:
		payloadPb, ok := payload.Proto().(*pb.ExecutionPayload)
		if !ok {
			return nil, errors.New("execution data must be a Bellatrix or Capella execution payload")
		}
		err := s.rpcClient.CallContext(ctx, result, NewPayloadMethod, payloadPb)
		if err != nil {
			return nil, handleRPCError(err)
		}
	case *pb.ExecutionPayloadCapella:
		payloadPb, ok := payload.Proto().(*pb.ExecutionPayloadCapella)
		if !ok {
			return nil, errors.New("execution data must be a Capella execution payload")
		}
		err := s.rpcClient.CallContext(ctx, result, NewPayloadMethodV2, payloadPb)
		if err != nil {
			return nil, handleRPCError(err)
		}
	case *pb.ExecutionPayloadDeneb:
		payloadPb, ok := payload.Proto().(*pb.ExecutionPayloadDeneb)
		if !ok {
			return nil, errors.New("execution data must be a Deneb execution payload")
		}
		if executionRequests == nil {
			err := s.rpcClient.CallContext(ctx, result, NewPayloadMethodV3, payloadPb, versionedHashes, parentBlockRoot)
			if err != nil {
				return nil, handleRPCError(err)
			}
		} else {
			flattenedRequests, err := pb.EncodeExecutionRequests(executionRequests)
			if err != nil {
				return nil, errors.Wrap(err, "failed to encode execution requests")
			}
			err = s.rpcClient.CallContext(ctx, result, NewPayloadMethodV4, payloadPb, versionedHashes, parentBlockRoot, flattenedRequests)
			if err != nil {
				return nil, handleRPCError(err)
			}
		}
	default:
		return nil, errors.New("unknown execution data type")
	}
	if result.ValidationError != "" {
		log.WithError(errors.New(result.ValidationError)).Error("Got a validation error in newPayload")
	}
	switch result.Status {
	case pb.PayloadStatus_INVALID_BLOCK_HASH:
		return nil, ErrInvalidBlockHashPayloadStatus
	case pb.PayloadStatus_ACCEPTED, pb.PayloadStatus_SYNCING:
		return nil, ErrAcceptedSyncingPayloadStatus
	case pb.PayloadStatus_INVALID:
		return result.LatestValidHash, ErrInvalidPayloadStatus
	case pb.PayloadStatus_VALID:
		return result.LatestValidHash, nil
	default:
		return nil, ErrUnknownPayloadStatus
	}
}

// ForkchoiceUpdated calls the engine_forkchoiceUpdatedV1 method via JSON-RPC.
func (s *Service) ForkchoiceUpdated(
	ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer,
) (*pb.PayloadIDBytes, []byte, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ForkchoiceUpdated")
	defer span.End()
	start := time.Now()
	defer func() {
		forkchoiceUpdatedLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()

	d := time.Now().Add(time.Duration(params.BeaconConfig().ExecutionEngineTimeoutValue) * time.Second)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()
	result := &ForkchoiceUpdatedResponse{}

	if attrs == nil {
		return nil, nil, errors.New("nil payload attributer")
	}
	switch attrs.Version() {
	case version.Bellatrix:
		a, err := attrs.PbV1()
		if err != nil {
			return nil, nil, err
		}
		err = s.rpcClient.CallContext(ctx, result, ForkchoiceUpdatedMethod, state, a)
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	case version.Capella:
		a, err := attrs.PbV2()
		if err != nil {
			return nil, nil, err
		}
		err = s.rpcClient.CallContext(ctx, result, ForkchoiceUpdatedMethodV2, state, a)
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	case version.Deneb, version.Electra:
		a, err := attrs.PbV3()
		if err != nil {
			return nil, nil, err
		}
		err = s.rpcClient.CallContext(ctx, result, ForkchoiceUpdatedMethodV3, state, a)
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	default:
		return nil, nil, fmt.Errorf("unknown payload attribute version: %v", attrs.Version())
	}

	if result.Status == nil {
		return nil, nil, ErrNilResponse
	}
	if result.ValidationError != "" {
		log.WithError(errors.New(result.ValidationError)).Error("Got a validation error in forkChoiceUpdated")
	}
	resp := result.Status
	switch resp.Status {
	case pb.PayloadStatus_SYNCING:
		return nil, nil, ErrAcceptedSyncingPayloadStatus
	case pb.PayloadStatus_INVALID:
		return nil, resp.LatestValidHash, ErrInvalidPayloadStatus
	case pb.PayloadStatus_VALID:
		return result.PayloadId, resp.LatestValidHash, nil
	default:
		return nil, nil, ErrUnknownPayloadStatus
	}
}

func getPayloadMethodAndMessage(slot primitives.Slot) (string, proto.Message) {
	pe := slots.ToEpoch(slot)
	if pe >= params.BeaconConfig().ElectraForkEpoch {
		return GetPayloadMethodV4, &pb.ExecutionBundleElectra{}
	}
	if pe >= params.BeaconConfig().DenebForkEpoch {
		return GetPayloadMethodV3, &pb.ExecutionPayloadDenebWithValueAndBlobsBundle{}
	}
	if pe >= params.BeaconConfig().CapellaForkEpoch {
		return GetPayloadMethodV2, &pb.ExecutionPayloadCapellaWithValue{}
	}
	return GetPayloadMethod, &pb.ExecutionPayload{}
}

// GetPayload calls the engine_getPayloadVX method via JSON-RPC.
// It returns the execution data as well as the blobs bundle.
func (s *Service) GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetPayload")
	defer span.End()
	start := time.Now()
	defer func() {
		getPayloadLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()
	d := time.Now().Add(defaultEngineTimeout)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()

	method, result := getPayloadMethodAndMessage(slot)
	err := s.rpcClient.CallContext(ctx, result, method, pb.PayloadIDBytes(payloadId))
	if err != nil {
		return nil, handleRPCError(err)
	}
	res, err := blocks.NewGetPayloadResponse(result)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) ExchangeCapabilities(ctx context.Context) ([]string, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ExchangeCapabilities")
	defer span.End()

	var result []string
	err := s.rpcClient.CallContext(ctx, &result, ExchangeCapabilities, supportedEngineEndpoints)
	if err != nil {
		return nil, handleRPCError(err)
	}

	var unsupported []string
	for _, s1 := range supportedEngineEndpoints {
		supported := false
		for _, s2 := range result {
			if s1 == s2 {
				supported = true
				break
			}
		}
		if !supported {
			unsupported = append(unsupported, s1)
		}
	}
	if len(unsupported) != 0 {
		log.Warnf("Please update client, detected the following unsupported engine methods: %s", unsupported)
	}
	return result, handleRPCError(err)
}

// GetTerminalBlockHash returns the valid terminal block hash based on total difficulty.
//
// Spec code:
// def get_pow_block_at_terminal_total_difficulty(pow_chain: Dict[Hash32, PowBlock]) -> Optional[PowBlock]:
//
//	# `pow_chain` abstractly represents all blocks in the PoW chain
//	for block in pow_chain:
//	    parent = pow_chain[block.parent_hash]
//	    block_reached_ttd = block.total_difficulty >= TERMINAL_TOTAL_DIFFICULTY
//	    parent_reached_ttd = parent.total_difficulty >= TERMINAL_TOTAL_DIFFICULTY
//	    if block_reached_ttd and not parent_reached_ttd:
//	        return block
//
//	return None
func (s *Service) GetTerminalBlockHash(ctx context.Context, transitionTime uint64) ([]byte, bool, error) {
	ttd := new(big.Int)
	ttd.SetString(params.BeaconConfig().TerminalTotalDifficulty, 10)
	terminalTotalDifficulty, overflows := uint256.FromBig(ttd)
	if overflows {
		return nil, false, errors.New("could not convert terminal total difficulty to uint256")
	}
	blk, err := s.LatestExecutionBlock(ctx)
	if err != nil {
		return nil, false, errors.Wrap(err, "could not get latest execution block")
	}
	if blk == nil {
		return nil, false, errors.New("latest execution block is nil")
	}

	for {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		currentTotalDifficulty, err := tDStringToUint256(blk.TotalDifficulty)
		if err != nil {
			return nil, false, errors.Wrap(err, "could not convert total difficulty to uint256")
		}
		blockReachedTTD := currentTotalDifficulty.Cmp(terminalTotalDifficulty) >= 0

		parentHash := blk.ParentHash
		if parentHash == params.BeaconConfig().ZeroHash {
			return nil, false, nil
		}
		parentBlk, err := s.ExecutionBlockByHash(ctx, parentHash, false /* no txs */)
		if err != nil {
			return nil, false, errors.Wrap(err, "could not get parent execution block")
		}
		if parentBlk == nil {
			return nil, false, errors.New("parent execution block is nil")
		}

		if blockReachedTTD {
			parentTotalDifficulty, err := tDStringToUint256(parentBlk.TotalDifficulty)
			if err != nil {
				return nil, false, errors.Wrap(err, "could not convert total difficulty to uint256")
			}

			// If terminal block has time same timestamp or greater than transition time,
			// then the node violates the invariant that a block's timestamp must be
			// greater than its parent's timestamp. Execution layer will reject
			// a fcu call with such payload attributes. It's best that we return `None` in this a case.
			parentReachedTTD := parentTotalDifficulty.Cmp(terminalTotalDifficulty) >= 0
			if !parentReachedTTD {
				if blk.Time >= transitionTime {
					return nil, false, nil
				}

				log.WithFields(logrus.Fields{
					"number":   blk.Number,
					"hash":     fmt.Sprintf("%#x", bytesutil.Trunc(blk.Hash[:])),
					"td":       blk.TotalDifficulty,
					"parentTd": parentBlk.TotalDifficulty,
					"ttd":      terminalTotalDifficulty,
				}).Info("Retrieved terminal block hash")
				return blk.Hash[:], true, nil
			}
		} else {
			return nil, false, nil
		}
		blk = parentBlk
	}
}

// LatestExecutionBlock fetches the latest execution engine block by calling
// eth_blockByNumber via JSON-RPC.
func (s *Service) LatestExecutionBlock(ctx context.Context) (*pb.ExecutionBlock, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.LatestExecutionBlock")
	defer span.End()

	result := &pb.ExecutionBlock{}
	err := s.rpcClient.CallContext(
		ctx,
		result,
		BlockByNumberMethod,
		"latest",
		false, /* no full transaction objects */
	)
	return result, handleRPCError(err)
}

// ExecutionBlockByHash fetches an execution engine block by hash by calling
// eth_blockByHash via JSON-RPC.
func (s *Service) ExecutionBlockByHash(ctx context.Context, hash common.Hash, withTxs bool) (*pb.ExecutionBlock, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ExecutionBlockByHash")
	defer span.End()
	result := &pb.ExecutionBlock{}
	err := s.rpcClient.CallContext(ctx, result, BlockByHashMethod, hash, withTxs)
	return result, handleRPCError(err)
}

// ExecutionBlocksByHashes fetches a batch of execution engine blocks by hash by calling
// eth_blockByHash via JSON-RPC.
func (s *Service) ExecutionBlocksByHashes(ctx context.Context, hashes []common.Hash, withTxs bool) ([]*pb.ExecutionBlock, error) {
	_, span := trace.StartSpan(ctx, "powchain.engine-api-client.ExecutionBlocksByHashes")
	defer span.End()
	numOfHashes := len(hashes)
	elems := make([]gethRPC.BatchElem, 0, numOfHashes)
	execBlks := make([]*pb.ExecutionBlock, 0, numOfHashes)
	if numOfHashes == 0 {
		return execBlks, nil
	}
	for _, h := range hashes {
		blk := &pb.ExecutionBlock{}
		newH := h
		elems = append(elems, gethRPC.BatchElem{
			Method: BlockByHashMethod,
			Args:   []interface{}{newH, withTxs},
			Result: blk,
			Error:  error(nil),
		})
		execBlks = append(execBlks, blk)
	}
	ioErr := s.rpcClient.BatchCall(elems)
	if ioErr != nil {
		return nil, ioErr
	}
	for _, e := range elems {
		if e.Error != nil {
			return nil, handleRPCError(e.Error)
		}
	}
	return execBlks, nil
}

// HeaderByHash returns the relevant header details for the provided block hash.
func (s *Service) HeaderByHash(ctx context.Context, hash common.Hash) (*types.HeaderInfo, error) {
	var hdr *types.HeaderInfo
	err := s.rpcClient.CallContext(ctx, &hdr, BlockByHashMethod, hash, false /* no transactions */)
	if err == nil && hdr == nil {
		err = ethereum.NotFound
	}
	return hdr, err
}

// HeaderByNumber returns the relevant header details for the provided block number.
func (s *Service) HeaderByNumber(ctx context.Context, number *big.Int) (*types.HeaderInfo, error) {
	var hdr *types.HeaderInfo
	err := s.rpcClient.CallContext(ctx, &hdr, BlockByNumberMethod, toBlockNumArg(number), false /* no transactions */)
	if err == nil && hdr == nil {
		err = ethereum.NotFound
	}
	return hdr, err
}

// GetBlobs returns the blob and proof from the execution engine for the given versioned hashes.
func (s *Service) GetBlobs(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetBlobs")
	defer span.End()
	// If the execution engine does not support `GetBlobsV1`, return early to prevent encountering an error later.
	if !s.capabilityCache.has(GetBlobsV1) {
		return nil, nil
	}

	result := make([]*pb.BlobAndProof, len(versionedHashes))
	err := s.rpcClient.CallContext(ctx, &result, GetBlobsV1, versionedHashes)
	return result, handleRPCError(err)
}

// ReconstructFullBlock takes in a blinded beacon block and reconstructs
// a beacon block with a full execution payload via the engine API.
func (s *Service) ReconstructFullBlock(
	ctx context.Context, blindedBlock interfaces.ReadOnlySignedBeaconBlock,
) (interfaces.SignedBeaconBlock, error) {
	reconstructed, err := s.ReconstructFullBellatrixBlockBatch(ctx, []interfaces.ReadOnlySignedBeaconBlock{blindedBlock})
	if err != nil {
		return nil, err
	}
	if len(reconstructed) != 1 {
		return nil, errors.Errorf("could not retrieve the correct number of payload bodies: wanted 1 but got %d", len(reconstructed))
	}
	return reconstructed[0], nil
}

// ReconstructFullBellatrixBlockBatch takes in a batch of blinded beacon blocks and reconstructs
// them with a full execution payload for each block via the engine API.
func (s *Service) ReconstructFullBellatrixBlockBatch(
	ctx context.Context, blindedBlocks []interfaces.ReadOnlySignedBeaconBlock,
) ([]interfaces.SignedBeaconBlock, error) {
	unb, err := reconstructBlindedBlockBatch(ctx, s.rpcClient, blindedBlocks)
	if err != nil {
		return nil, err
	}
	reconstructedExecutionPayloadCount.Add(float64(len(unb)))
	return unb, nil
}

// ReconstructBlobSidecars reconstructs the verified blob sidecars for a given beacon block.
// It retrieves the KZG commitments from the block body, fetches the associated blobs and proofs,
// and constructs the corresponding verified read-only blob sidecars.
//
// The 'exists' argument is a boolean list (must be the same length as body.BlobKzgCommitments), where each element corresponds to whether a
// particular blob sidecar already exists. If exists[i] is true, the blob for the i-th KZG commitment
// has already been retrieved and does not need to be fetched again from the execution layer (EL).
//
// For example:
//   - len(block.Body().BlobKzgCommitments()) == 6
//   - If exists = [true, false, true, false, true, false], the function will fetch the blobs
//     associated with indices 1, 3, and 5 (since those are marked as non-existent).
//   - If exists = [false ... x 6], the function will attempt to fetch all blobs.
//
// Only the blobs that do not already exist (where exists[i] is false) are fetched using the KZG commitments from block body.
func (s *Service) ReconstructBlobSidecars(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte, exists []bool) ([]blocks.VerifiedROBlob, error) {
	blockBody := block.Block().Body()
	kzgCommitments, err := blockBody.BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "could not get blob KZG commitments")
	}
	if len(kzgCommitments) != len(exists) {
		return nil, fmt.Errorf("mismatched lengths: KZG commitments %d, exists %d", len(kzgCommitments), len(exists))
	}

	// Collect KZG hashes for non-existing blobs
	var kzgHashes []common.Hash
	for i, commitment := range kzgCommitments {
		if !exists[i] {
			kzgHashes = append(kzgHashes, primitives.ConvertKzgCommitmentToVersionedHash(commitment))
		}
	}
	if len(kzgHashes) == 0 {
		return nil, nil
	}

	// Fetch blobs from EL
	blobs, err := s.GetBlobs(ctx, kzgHashes)
	if err != nil {
		return nil, errors.Wrap(err, "could not get blobs")
	}
	if len(blobs) == 0 {
		return nil, nil
	}

	header, err := block.Header()
	if err != nil {
		return nil, errors.Wrap(err, "could not get header")
	}

	// Reconstruct verified blob sidecars
	var verifiedBlobs []blocks.VerifiedROBlob
	for i, blobIndex := 0, 0; i < len(kzgCommitments); i++ {
		if exists[i] {
			continue
		}

		if blobIndex >= len(blobs) || blobs[blobIndex] == nil {
			blobIndex++
			continue
		}
		blob := blobs[blobIndex]
		blobIndex++

		proof, err := blocks.MerkleProofKZGCommitment(blockBody, i)
		if err != nil {
			log.WithError(err).WithField("index", i).Error("failed to get Merkle proof for KZG commitment")
			continue
		}
		sidecar := &ethpb.BlobSidecar{
			Index:                    uint64(i),
			Blob:                     blob.Blob,
			KzgCommitment:            kzgCommitments[i],
			KzgProof:                 blob.KzgProof,
			SignedBlockHeader:        header,
			CommitmentInclusionProof: proof,
		}

		roBlob, err := blocks.NewROBlobWithRoot(sidecar, blockRoot)
		if err != nil {
			log.WithError(err).WithField("index", i).Error("failed to create RO blob with root")
			continue
		}

		// Verify the sidecar KZG proof
		v := s.blobVerifier(roBlob, verification.ELMemPoolRequirements)
		if err := v.SidecarKzgProofVerified(); err != nil {
			log.WithError(err).WithField("index", i).Error("failed to verify KZG proof for sidecar")
			continue
		}

		verifiedBlob, err := v.VerifiedROBlob()
		if err != nil {
			log.WithError(err).WithField("index", i).Error("failed to verify RO blob")
			continue
		}

		verifiedBlobs = append(verifiedBlobs, verifiedBlob)
	}

	return verifiedBlobs, nil
}

func fullPayloadFromPayloadBody(
	header interfaces.ExecutionData, body *pb.ExecutionPayloadBody, bVersion int,
) (interfaces.ExecutionData, error) {
	if header == nil || header.IsNil() || body == nil {
		return nil, errors.New("execution block and header cannot be nil")
	}

	switch bVersion {
	case version.Bellatrix:
		return blocks.WrappedExecutionPayload(&pb.ExecutionPayload{
			ParentHash:    header.ParentHash(),
			FeeRecipient:  header.FeeRecipient(),
			StateRoot:     header.StateRoot(),
			ReceiptsRoot:  header.ReceiptsRoot(),
			LogsBloom:     header.LogsBloom(),
			PrevRandao:    header.PrevRandao(),
			BlockNumber:   header.BlockNumber(),
			GasLimit:      header.GasLimit(),
			GasUsed:       header.GasUsed(),
			Timestamp:     header.Timestamp(),
			ExtraData:     header.ExtraData(),
			BaseFeePerGas: header.BaseFeePerGas(),
			BlockHash:     header.BlockHash(),
			Transactions:  pb.RecastHexutilByteSlice(body.Transactions),
		})
	case version.Capella:
		return blocks.WrappedExecutionPayloadCapella(&pb.ExecutionPayloadCapella{
			ParentHash:    header.ParentHash(),
			FeeRecipient:  header.FeeRecipient(),
			StateRoot:     header.StateRoot(),
			ReceiptsRoot:  header.ReceiptsRoot(),
			LogsBloom:     header.LogsBloom(),
			PrevRandao:    header.PrevRandao(),
			BlockNumber:   header.BlockNumber(),
			GasLimit:      header.GasLimit(),
			GasUsed:       header.GasUsed(),
			Timestamp:     header.Timestamp(),
			ExtraData:     header.ExtraData(),
			BaseFeePerGas: header.BaseFeePerGas(),
			BlockHash:     header.BlockHash(),
			Transactions:  pb.RecastHexutilByteSlice(body.Transactions),
			Withdrawals:   body.Withdrawals,
		}) // We can't get the block value and don't care about the block value for this instance
	case version.Deneb, version.Electra:
		ebg, err := header.ExcessBlobGas()
		if err != nil {
			return nil, errors.Wrap(err, "unable to extract ExcessBlobGas attribute from execution payload header")
		}
		bgu, err := header.BlobGasUsed()
		if err != nil {
			return nil, errors.Wrap(err, "unable to extract BlobGasUsed attribute from execution payload header")
		}
		return blocks.WrappedExecutionPayloadDeneb(
			&pb.ExecutionPayloadDeneb{
				ParentHash:    header.ParentHash(),
				FeeRecipient:  header.FeeRecipient(),
				StateRoot:     header.StateRoot(),
				ReceiptsRoot:  header.ReceiptsRoot(),
				LogsBloom:     header.LogsBloom(),
				PrevRandao:    header.PrevRandao(),
				BlockNumber:   header.BlockNumber(),
				GasLimit:      header.GasLimit(),
				GasUsed:       header.GasUsed(),
				Timestamp:     header.Timestamp(),
				ExtraData:     header.ExtraData(),
				BaseFeePerGas: header.BaseFeePerGas(),
				BlockHash:     header.BlockHash(),
				Transactions:  pb.RecastHexutilByteSlice(body.Transactions),
				Withdrawals:   body.Withdrawals,
				ExcessBlobGas: ebg,
				BlobGasUsed:   bgu,
			}) // We can't get the block value and don't care about the block value for this instance
	default:
		return nil, fmt.Errorf("unknown execution block version for payload %d", bVersion)
	}
}

// Handles errors received from the RPC server according to the specification.
func handleRPCError(err error) error {
	if err == nil {
		return nil
	}
	if isTimeout(err) {
		return ErrHTTPTimeout
	}
	var e gethRPC.Error
	ok := errors.As(err, &e)
	if !ok {
		if strings.Contains(err.Error(), "401 Unauthorized") {
			log.Error("HTTP authentication to your execution client is not working. Please ensure " +
				"you are setting a correct value for the --jwt-secret flag in Prysm, or use an IPC connection if on " +
				"the same machine. Please see our documentation for more information on authenticating connections " +
				"here https://docs.prylabs.network/docs/execution-node/authentication")
			return fmt.Errorf("could not authenticate connection to execution client: %w", err)
		}
		return errors.Wrapf(err, "got an unexpected error in JSON-RPC response")
	}
	switch e.ErrorCode() {
	case -32700:
		errParseCount.Inc()
		return ErrParse
	case -32600:
		errInvalidRequestCount.Inc()
		return ErrInvalidRequest
	case -32601:
		errMethodNotFoundCount.Inc()
		return ErrMethodNotFound
	case -32602:
		errInvalidParamsCount.Inc()
		return ErrInvalidParams
	case -32603:
		errInternalCount.Inc()
		return ErrInternal
	case -38001:
		errUnknownPayloadCount.Inc()
		return ErrUnknownPayload
	case -38002:
		errInvalidForkchoiceStateCount.Inc()
		return ErrInvalidForkchoiceState
	case -38003:
		errInvalidPayloadAttributesCount.Inc()
		return ErrInvalidPayloadAttributes
	case -38004:
		errRequestTooLargeCount.Inc()
		return ErrRequestTooLarge
	case -32000:
		errServerErrorCount.Inc()
		// Only -32000 status codes are data errors in the RPC specification.
		var errWithData gethRPC.DataError
		ok := errors.As(err, &errWithData)
		if !ok {
			return errors.Wrapf(err, "got an unexpected error in JSON-RPC response")
		}
		return errors.Wrapf(ErrServer, "%v", errWithData.Error())
	default:
		return err
	}
}

// ErrHTTPTimeout returns true if the error is a http.Client timeout error.
var ErrHTTPTimeout = errors.New("timeout from http.Client")

type httpTimeoutError interface {
	Error() string
	Timeout() bool
}

func isTimeout(e error) bool {
	var t httpTimeoutError
	ok := errors.As(e, &t)
	return ok && t.Timeout()
}

func tDStringToUint256(td string) (*uint256.Int, error) {
	b, err := hexutil.DecodeBig(td)
	if err != nil {
		return nil, err
	}
	i, overflows := uint256.FromBig(b)
	if overflows {
		return nil, errors.New("total difficulty overflowed")
	}
	return i, nil
}

func buildEmptyExecutionPayload(v int) (proto.Message, error) {
	switch v {
	case version.Bellatrix:
		return &pb.ExecutionPayload{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			ExtraData:     make([]byte, 0),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
		}, nil
	case version.Capella:
		return &pb.ExecutionPayloadCapella{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			ExtraData:     make([]byte, 0),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			Withdrawals:   make([]*pb.Withdrawal, 0),
		}, nil
	case version.Deneb, version.Electra:
		return &pb.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			ExtraData:     make([]byte, 0),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			Withdrawals:   make([]*pb.Withdrawal, 0),
		}, nil
	default:
		return nil, errors.Wrapf(ErrUnsupportedVersion, "version=%s", version.String(v))
	}
}

func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	pending := big.NewInt(-1)
	if number.Cmp(pending) == 0 {
		return "pending"
	}
	finalized := big.NewInt(int64(gethRPC.FinalizedBlockNumber))
	if number.Cmp(finalized) == 0 {
		return "finalized"
	}
	safe := big.NewInt(int64(gethRPC.SafeBlockNumber))
	if number.Cmp(safe) == 0 {
		return "safe"
	}
	return hexutil.EncodeBig(number)
}
