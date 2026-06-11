package execution

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
)

var errInvalidPayloadBodyResponse = errors.New("engine api payload body response is invalid")

// Reconstructor defines a service responsible for reconstructing full beacon chain objects by utilizing the execution API and making requests through the execution client.
type Reconstructor interface {
	ReconstructFullBlock(
		ctx context.Context, blindedBlock interfaces.ReadOnlySignedBeaconBlock,
	) (interfaces.SignedBeaconBlock, error)
	ReconstructFullBellatrixBlockBatch(
		ctx context.Context, blindedBlocks []interfaces.ReadOnlySignedBeaconBlock,
	) ([]interfaces.SignedBeaconBlock, error)
	ReconstructFullGloasExecutionPayloadsByHash(
		ctx context.Context, blockHashes [][32]byte,
	) (map[[32]byte]*pb.ExecutionPayloadGloas, error)
	ReconstructBlobSidecars(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [fieldparams.RootLength]byte, hi func(uint64) bool) ([]blocks.VerifiedROBlob, error)
	ConstructDataColumnSidecars(ctx context.Context, populator peerdas.ConstructionPopulator) ([]blocks.VerifiedRODataColumn, error)
	ReconstructExecutionPayloadEnvelope(ctx context.Context, envelope *ethpb.SignedBlindedExecutionPayloadEnvelope) (*ethpb.SignedExecutionPayloadEnvelope, error)
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
	GetClientVersionV1(ctx context.Context) ([]*structs.ClientVersionV1, error)
}

var ErrEmptyBlockHash = errors.New("Block hash is empty 0x0000...")

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
	unb, err := reconstructBlindedBlockBatch(ctx, s.engine(), blindedBlocks)
	if err != nil {
		return nil, err
	}
	reconstructedExecutionPayloadCount.Add(float64(len(unb)))
	return unb, nil
}

// ReconstructExecutionPayloadEnvelope reconstructs a full Gloas envelope from a blinded envelope.
func (s *Service) ReconstructExecutionPayloadEnvelope(
	ctx context.Context, envelope *ethpb.SignedBlindedExecutionPayloadEnvelope,
) (*ethpb.SignedExecutionPayloadEnvelope, error) {
	if envelope == nil || envelope.Message == nil {
		return nil, errors.New("nil blinded execution payload envelope")
	}
	blockHash := bytesutil.ToBytes32(envelope.Message.BlockHash)
	payloads, err := s.ReconstructFullGloasExecutionPayloadsByHash(ctx, [][32]byte{blockHash})
	if err != nil {
		return nil, errors.Wrap(err, "could not reconstruct execution payload")
	}
	payload, ok := payloads[blockHash]
	if !ok || payload == nil {
		return nil, errors.New("execution payload not found")
	}
	return &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload:               payload,
			ExecutionRequests:     envelope.Message.ExecutionRequests,
			BuilderIndex:          envelope.Message.BuilderIndex,
			BeaconBlockRoot:       envelope.Message.BeaconBlockRoot,
			ParentBeaconBlockRoot: envelope.Message.ParentBeaconBlockRoot,
		},
		Signature: envelope.Signature,
	}, nil
}

// ReconstructFullGloasExecutionPayloadsByHash reconstructs full Gloas payloads from EL data.
func (s *Service) ReconstructFullGloasExecutionPayloadsByHash(
	ctx context.Context, blockHashes [][32]byte,
) (map[[32]byte]*pb.ExecutionPayloadGloas, error) {
	payloads := make(map[[32]byte]*pb.ExecutionPayloadGloas, len(blockHashes))
	if len(blockHashes) == 0 {
		return payloads, nil
	}

	uniqueSet := make(map[[32]byte]struct{}, len(blockHashes))
	uniqueHashes := make([][32]byte, 0, len(blockHashes))
	for i := range blockHashes {
		h := blockHashes[i]
		if _, ok := uniqueSet[h]; ok {
			continue
		}
		uniqueSet[h] = struct{}{}
		uniqueHashes = append(uniqueHashes, h)
	}

	requestHashes := make([]common.Hash, 0, len(uniqueHashes))
	for i := range uniqueHashes {
		if uniqueHashes[i] == params.BeaconConfig().ZeroHash {
			empty, err := EmptyExecutionPayload(version.Gloas)
			if err != nil {
				return nil, err
			}
			payloads[uniqueHashes[i]] = empty.(*pb.ExecutionPayloadGloas)
			continue
		}
		requestHashes = append(requestHashes, uniqueHashes[i])
	}

	if len(requestHashes) == 0 {
		return payloads, nil
	}

	var execBlocks []*pb.ExecutionBlock
	bodiesV2 := make([]*pb.ExecutionPayloadBodyV2, 0)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		blks, err := s.ExecutionBlocksByHashes(gctx, requestHashes, false)
		if err != nil {
			return errors.Wrap(err, "could not fetch execution blocks by hash")
		}
		execBlocks = blks
		return nil
	})
	g.Go(func() error {
		if err := s.rpcClient.CallContext(gctx, &bodiesV2, GetPayloadBodiesByHashV2, requestHashes); err != nil {
			return errors.Wrap(err, "could not fetch payload bodies V2 by hash")
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if len(bodiesV2) != len(requestHashes) {
		return nil, errors.Errorf("payload bodies V2 count mismatch: got %d, want %d", len(bodiesV2), len(requestHashes))
	}

	for i, h := range requestHashes {
		blk := execBlocks[i]
		payload, err := gloasPayloadFromExecutionBlock(h, blk)
		if err != nil {
			return nil, err
		}
		if bodiesV2[i] != nil {
			payload.Transactions = pb.RecastHexutilByteSlice(bodiesV2[i].Transactions)
			payload.Withdrawals = bodiesV2[i].Withdrawals
			if bodiesV2[i].BlockAccessList != nil {
				payload.BlockAccessList = *bodiesV2[i].BlockAccessList
			}
		}
		payloads[h] = payload
	}

	return payloads, nil
}

// gloasPayloadFromExecutionBlock extracts header fields from an execution block.
func gloasPayloadFromExecutionBlock(
	requestedHash [32]byte, blk *pb.ExecutionBlock,
) (*pb.ExecutionPayloadGloas, error) {
	if blk == nil {
		return nil, errors.New("execution block not found")
	}
	if blk.Hash == (common.Hash{}) || blk.Hash != requestedHash {
		return nil, errors.New("execution block hash mismatch")
	}
	if blk.Number == nil {
		return nil, errors.New("execution block number is nil")
	}
	if blk.BaseFee == nil {
		return nil, errors.New("execution block base fee is nil")
	}

	if blk.BlobGasUsed == nil {
		return nil, errors.New("execution block blob gas used is nil")
	}
	if blk.ExcessBlobGas == nil {
		return nil, errors.New("execution block excess blob gas is nil")
	}
	if blk.SlotNumber == nil {
		return nil, errors.New("execution block slot number is nil")
	}

	return &pb.ExecutionPayloadGloas{
		ParentHash:      blk.ParentHash.Bytes(),
		FeeRecipient:    blk.Coinbase.Bytes(),
		StateRoot:       blk.Root.Bytes(),
		ReceiptsRoot:    blk.ReceiptHash.Bytes(),
		LogsBloom:       blk.Bloom.Bytes(),
		PrevRandao:      blk.MixDigest.Bytes(),
		BlockNumber:     blk.Number.Uint64(),
		GasLimit:        blk.GasLimit,
		GasUsed:         blk.GasUsed,
		Timestamp:       blk.Time,
		ExtraData:       blk.Extra,
		BaseFeePerGas:   bytesutil.PadTo(bytesutil.ReverseByteOrder(blk.BaseFee.Bytes()), fieldparams.RootLength),
		BlockHash:       blk.Hash.Bytes(),
		BlobGasUsed:     *blk.BlobGasUsed,
		ExcessBlobGas:   *blk.ExcessBlobGas,
		SlotNumber:      primitives.Slot(*blk.SlotNumber),
		BlockAccessList: blk.BlockAccessList,
	}, nil
}

// ReconstructBlobSidecars reconstructs the verified blob sidecars for a given beacon block.
// It retrieves the KZG commitments from the block body, fetches the associated blobs and proofs,
// and constructs the corresponding verified read-only blob sidecars.
//
// The 'hasIndex' argument is a function returns true if the given uint64 blob index already exists on disc.
// Only the blobs that do not already exist (where hasIndex(i) is false)
// will be fetched from the execution engine using the KZG commitments from block body.
func (s *Service) ReconstructBlobSidecars(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte, hasIndex func(uint64) bool) ([]blocks.VerifiedROBlob, error) {
	blockBody := block.Block().Body()
	kzgCommitments, err := blockBody.BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "could not get blob KZG commitments")
	}

	// Collect KZG hashes for non-existing blobs
	var kzgHashes []common.Hash
	var kzgIndexes []int
	for i, commitment := range kzgCommitments {
		if !hasIndex(uint64(i)) {
			kzgHashes = append(kzgHashes, primitives.ConvertKzgCommitmentToVersionedHash(commitment))
			kzgIndexes = append(kzgIndexes, i)
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
	for i := 0; i < len(kzgHashes); i++ {
		if blobs[i] == nil {
			continue
		}
		blob := blobs[i]
		blobIndex := kzgIndexes[i]
		proof, err := blocks.MerkleProofKZGCommitment(blockBody, blobIndex)
		if err != nil {
			log.WithError(err).WithField("index", blobIndex).Error("Failed to get Merkle proof for KZG commitment")
			continue
		}
		sidecar := &ethpb.BlobSidecar{
			Index:                    uint64(blobIndex),
			Blob:                     blob.Blob,
			KzgCommitment:            kzgCommitments[blobIndex],
			KzgProof:                 blob.KzgProof,
			SignedBlockHeader:        header,
			CommitmentInclusionProof: proof,
		}

		roBlob, err := blocks.NewROBlobWithRoot(sidecar, blockRoot)
		if err != nil {
			log.WithError(err).WithField("index", blobIndex).Error("Failed to create RO blob with root")
			continue
		}

		v := s.blobVerifier(roBlob, verification.ELMemPoolRequirements)
		verifiedBlob, err := v.VerifiedROBlob()
		if err != nil {
			log.WithError(err).WithField("index", blobIndex).Error("Failed to verify RO blob")
			continue
		}

		verifiedBlobs = append(verifiedBlobs, verifiedBlob)
	}

	return verifiedBlobs, nil
}

func (s *Service) ConstructDataColumnSidecars(ctx context.Context, populator peerdas.ConstructionPopulator) ([]blocks.VerifiedRODataColumn, error) {
	root := populator.Root()

	// Fetch cells and proofs from the execution client using the KZG commitments from the sidecar.
	commitments, err := populator.Commitments()
	if err != nil {
		return nil, wrapWithBlockRoot(err, root, "commitments")
	}

	cellsPerBlob, proofsPerBlob, err := s.fetchCellsAndProofsFromExecution(ctx, commitments)
	if err != nil {
		return nil, wrapWithBlockRoot(err, root, "fetch cells and proofs from execution client")
	}

	// Return early if nothing is returned from the EL.
	if len(cellsPerBlob) == 0 {
		return nil, nil
	}

	// Construct data column sidears from the signed block and cells and proofs.
	roSidecars, err := peerdas.DataColumnSidecars(cellsPerBlob, proofsPerBlob, populator)
	if err != nil {
		return nil, wrapWithBlockRoot(err, populator.Root(), "data column sidcars from column sidecar")
	}

	// Upgrade the sidecars to verified sidecars.
	// We trust the execution layer we are connected to, so we can upgrade the sidecar into a verified one.
	verifiedROSidecars := upgradeSidecarsToVerifiedSidecars(roSidecars)

	return verifiedROSidecars, nil
}

// fetchCellsAndProofsFromExecution fetches cells and proofs from the execution client (using engine_getBlobsV2 execution API method)
func (s *Service) fetchCellsAndProofsFromExecution(ctx context.Context, kzgCommitments [][]byte) ([][]kzg.Cell, [][]kzg.Proof, error) {
	// Collect KZG hashes for all blobs.
	versionedHashes := make([]common.Hash, 0, len(kzgCommitments))
	for _, commitment := range kzgCommitments {
		versionedHash := primitives.ConvertKzgCommitmentToVersionedHash(commitment)
		versionedHashes = append(versionedHashes, versionedHash)
	}

	// Fetch all blobsAndCellsProofs from the execution client.
	blobAndProofV2s, err := s.GetBlobsV2(ctx, versionedHashes)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "get blobs V2")
	}

	// Return early if nothing is returned from the EL.
	if len(blobAndProofV2s) == 0 {
		return nil, nil, nil
	}

	// Compute cells and proofs from the blobs and cell proofs.
	cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromStructured(blobAndProofV2s)
	if err != nil {
		return nil, nil, errors.Wrap(err, "compute cells and proofs")
	}

	return cellsPerBlob, proofsPerBlob, nil
}

// upgradeSidecarsToVerifiedSidecars upgrades a list of data column sidecars into verified data column sidecars.
func upgradeSidecarsToVerifiedSidecars(roSidecars []blocks.RODataColumn) []blocks.VerifiedRODataColumn {
	verifiedRODataColumns := make([]blocks.VerifiedRODataColumn, 0, len(roSidecars))
	for _, roSidecar := range roSidecars {
		verifiedRODataColumn := blocks.NewVerifiedRODataColumn(roSidecar)
		verifiedRODataColumns = append(verifiedRODataColumns, verifiedRODataColumn)
	}

	return verifiedRODataColumns
}

func fullPayloadFromPayloadBody(
	header interfaces.ExecutionData, body *pb.ExecutionPayloadBody, bVersion int,
) (interfaces.ExecutionData, error) {
	if header == nil || header.IsNil() || body == nil {
		return nil, errors.New("execution block and header cannot be nil")
	}

	if bVersion >= version.Deneb {
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
	}

	if bVersion >= version.Capella {
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
	}

	if bVersion >= version.Bellatrix {
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
	}

	return nil, fmt.Errorf("unknown execution block version for payload %s", version.String(bVersion))
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

func EmptyExecutionPayload(v int) (proto.Message, error) {
	if v >= version.Gloas {
		return &pb.ExecutionPayloadGloas{
			ParentHash:      make([]byte, fieldparams.RootLength),
			FeeRecipient:    make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:       make([]byte, fieldparams.RootLength),
			ReceiptsRoot:    make([]byte, fieldparams.RootLength),
			LogsBloom:       make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:      make([]byte, fieldparams.RootLength),
			ExtraData:       make([]byte, 0),
			BaseFeePerGas:   make([]byte, fieldparams.RootLength),
			BlockHash:       make([]byte, fieldparams.RootLength),
			Transactions:    make([][]byte, 0),
			Withdrawals:     make([]*pb.Withdrawal, 0),
			BlockAccessList: make([]byte, 0),
		}, nil
	}

	if v >= version.Deneb {
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
	}

	if v >= version.Capella {
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
	}

	if v >= version.Bellatrix {
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
	}

	return nil, errors.Wrapf(ErrUnsupportedVersion, "version=%s", version.String(v))
}

func EmptyExecutionPayloadHeader(v int) (proto.Message, error) {
	if v >= version.Deneb {
		return &pb.ExecutionPayloadHeaderDeneb{
			ParentHash:       make([]byte, fieldparams.RootLength),
			FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:       make([]byte, fieldparams.RootLength),
			ExtraData:        make([]byte, 0),
			BaseFeePerGas:    make([]byte, fieldparams.RootLength),
			BlockHash:        make([]byte, fieldparams.RootLength),
			TransactionsRoot: make([]byte, fieldparams.RootLength),
			WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
		}, nil
	}

	if v >= version.Capella {
		return &pb.ExecutionPayloadHeaderCapella{
			ParentHash:       make([]byte, fieldparams.RootLength),
			FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:       make([]byte, fieldparams.RootLength),
			ExtraData:        make([]byte, 0),
			BaseFeePerGas:    make([]byte, fieldparams.RootLength),
			BlockHash:        make([]byte, fieldparams.RootLength),
			TransactionsRoot: make([]byte, fieldparams.RootLength),
			WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
		}, nil
	}

	if v >= version.Bellatrix {
		return &pb.ExecutionPayloadHeader{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			ExtraData:     make([]byte, 0),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
		}, nil
	}

	return nil, errors.Wrapf(ErrUnsupportedVersion, "version=%s", version.String(v))
}

// wrapWithBlockRoot returns a new error with the given block root.
func wrapWithBlockRoot(err error, blockRoot [fieldparams.RootLength]byte, message string) error {
	return errors.Wrap(err, fmt.Sprintf("%s for block %#x", message, blockRoot))
}
