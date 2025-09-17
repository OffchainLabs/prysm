package lookup

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/options"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

type BlockRootsNotFoundError struct {
	message string
}

func NewBlockRootsNotFoundError() *BlockRootsNotFoundError {
	return &BlockRootsNotFoundError{
		message: "no block roots returned from the database",
	}
}

func (e BlockRootsNotFoundError) Error() string {
	return e.message
}

// BlockIdParseError represents an error scenario where a block ID could not be parsed.
type BlockIdParseError struct {
	message string
}

// NewBlockIdParseError creates a new error instance.
func NewBlockIdParseError(reason error) BlockIdParseError {
	return BlockIdParseError{
		message: errors.Wrapf(reason, "could not parse block ID").Error(),
	}
}

// Error returns the underlying error message.
func (e BlockIdParseError) Error() string {
	return e.message
}

// Blocker is responsible for retrieving blocks.
type Blocker interface {
	Block(ctx context.Context, id []byte) (interfaces.ReadOnlySignedBeaconBlock, error)
	Blobs(ctx context.Context, id string, opts ...options.BlobsOption) ([]*blocks.VerifiedROBlob, *core.RpcError)
}

// BeaconDbBlocker is an implementation of Blocker. It retrieves blocks from the beacon chain database.
type BeaconDbBlocker struct {
	BeaconDB           db.ReadOnlyDatabase
	ChainInfoFetcher   blockchain.ChainInfoFetcher
	GenesisTimeFetcher blockchain.TimeFetcher
	BlobStorage        *filesystem.BlobStorage
	DataColumnStorage  *filesystem.DataColumnStorage
}

// Block returns the beacon block for a given identifier. The identifier can be one of:
//   - "head" (canonical head in node's view)
//   - "genesis"
//   - "finalized"
//   - "justified"
//   - <slot>
//   - <hex encoded block root with '0x' prefix>
//   - <block root>
func (p *BeaconDbBlocker) Block(ctx context.Context, id []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	var err error
	var blk interfaces.ReadOnlySignedBeaconBlock
	switch string(id) {
	case "head":
		blk, err = p.ChainInfoFetcher.HeadBlock(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not retrieve head block")
		}
	case "finalized":
		finalized := p.ChainInfoFetcher.FinalizedCheckpt()
		finalizedRoot := bytesutil.ToBytes32(finalized.Root)
		blk, err = p.BeaconDB.Block(ctx, finalizedRoot)
		if err != nil {
			return nil, errors.New("could not get finalized block from db")
		}
	case "genesis":
		blk, err = p.BeaconDB.GenesisBlock(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not retrieve genesis block")
		}
	default:
		if bytesutil.IsHex(id) {
			decoded, err := hexutil.Decode(string(id))
			if err != nil {
				e := NewBlockIdParseError(err)
				return nil, &e
			}
			blk, err = p.BeaconDB.Block(ctx, bytesutil.ToBytes32(decoded))
			if err != nil {
				return nil, errors.Wrap(err, "could not retrieve block")
			}
		} else if len(id) == 32 {
			blk, err = p.BeaconDB.Block(ctx, bytesutil.ToBytes32(id))
			if err != nil {
				return nil, errors.Wrap(err, "could not retrieve block")
			}
		} else {
			slot, err := strconv.ParseUint(string(id), 10, 64)
			if err != nil {
				e := NewBlockIdParseError(err)
				return nil, &e
			}
			blks, err := p.BeaconDB.BlocksBySlot(ctx, primitives.Slot(slot))
			if err != nil {
				return nil, errors.Wrapf(err, "could not retrieve blocks for slot %d", slot)
			}
			_, roots, err := p.BeaconDB.BlockRootsBySlot(ctx, primitives.Slot(slot))
			if err != nil {
				return nil, errors.Wrapf(err, "could not retrieve block roots for slot %d", slot)
			}
			numBlks := len(blks)
			if numBlks == 0 {
				return nil, nil
			}
			for i, b := range blks {
				canonical, err := p.ChainInfoFetcher.IsCanonical(ctx, roots[i])
				if err != nil {
					return nil, errors.Wrapf(err, "could not determine if block root is canonical")
				}
				if canonical {
					blk = b
					break
				}
			}
		}
	}
	return blk, nil
}

// Blobs returns the fetched blobs for a given block ID with configurable options.
// Options can specify either blob indices or versioned hashes for retrieval.
// The identifier can be one of:
//   - "head" (canonical head in node's view)
//   - "genesis"
//   - "finalized"
//   - "justified"
//   - <slot>
//   - <hex encoded block root with '0x' prefix>
//   - <block root>
//
// cases:
//   - no block, 404
//   - block exists, has commitments, inside retention period (greater of protocol- or user-specified) serve then w/ 200 unless we hit an error reading them.
//     we are technically not supposed to import a block to forkchoice unless we have the blobs, so the nuance here is if we can't find the file and we are inside the protocol-defined retention period, then it's actually a 500.
//   - block exists, has commitments, outside retention period (greater of protocol- or user-specified) - ie just like block exists, no commitment
func (p *BeaconDbBlocker) Blobs(ctx context.Context, id string, opts ...options.BlobsOption) ([]*blocks.VerifiedROBlob, *core.RpcError) {
	// Apply options
	cfg := &options.BlobsConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Resolve block ID to root
	var rootSlice []byte
	switch id {
	case "genesis":
		return nil, &core.RpcError{Err: errors.New("blobs are not supported for Phase 0 fork"), Reason: core.BadRequest}
	case "head":
		var err error
		rootSlice, err = p.ChainInfoFetcher.HeadRoot(ctx)
		if err != nil {
			return nil, &core.RpcError{Err: errors.Wrapf(err, "could not retrieve head root"), Reason: core.Internal}
		}
	case "finalized":
		fcp := p.ChainInfoFetcher.FinalizedCheckpt()
		if fcp == nil {
			return nil, &core.RpcError{Err: errors.New("received nil finalized checkpoint"), Reason: core.Internal}
		}
		rootSlice = fcp.Root
	case "justified":
		jcp := p.ChainInfoFetcher.CurrentJustifiedCheckpt()
		if jcp == nil {
			return nil, &core.RpcError{Err: errors.New("received nil justified checkpoint"), Reason: core.Internal}
		}
		rootSlice = jcp.Root
	default:
		if bytesutil.IsHex([]byte(id)) {
			var err error
			rootSlice, err = bytesutil.DecodeHexWithLength(id, fieldparams.RootLength)
			if err != nil {
				return nil, &core.RpcError{Err: NewBlockIdParseError(err), Reason: core.BadRequest}
			}
		} else {
			slot, err := strconv.ParseUint(id, 10, 64)
			if err != nil {
				return nil, &core.RpcError{Err: NewBlockIdParseError(err), Reason: core.BadRequest}
			}
			denebStart, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
			if err != nil {
				return nil, &core.RpcError{Err: errors.Wrap(err, "could not calculate Deneb start slot"), Reason: core.Internal}
			}
			if primitives.Slot(slot) < denebStart {
				return nil, &core.RpcError{Err: errors.New("blobs are not supported before Deneb fork"), Reason: core.BadRequest}
			}
			ok, roots, err := p.BeaconDB.BlockRootsBySlot(ctx, primitives.Slot(slot))
			if !ok {
				return nil, &core.RpcError{Err: fmt.Errorf("no block roots at slot %d", slot), Reason: core.NotFound}
			}
			if err != nil {
				return nil, &core.RpcError{Err: errors.Wrapf(err, "failed to get block roots for slot %d", slot), Reason: core.Internal}
			}
			rootSlice = roots[0][:]
			if len(roots) == 1 {
				break
			}
			for _, blockRoot := range roots {
				canonical, err := p.ChainInfoFetcher.IsCanonical(ctx, blockRoot)
				if err != nil {
					return nil, &core.RpcError{Err: errors.Wrapf(err, "could not determine if block %#x is canonical", blockRoot), Reason: core.Internal}
				}
				if canonical {
					rootSlice = blockRoot[:]
					break
				}
			}
		}
	}

	root := bytesutil.ToBytes32(rootSlice)

	roSignedBlock, err := p.BeaconDB.Block(ctx, root)
	if err != nil {
		return nil, &core.RpcError{Err: errors.Wrapf(err, "failed to retrieve block %#x from db", rootSlice), Reason: core.Internal}
	}

	if roSignedBlock == nil {
		return nil, &core.RpcError{Err: fmt.Errorf("block %#x not found in db", rootSlice), Reason: core.NotFound}
	}

	roBlock := roSignedBlock.Block()

	commitments, err := roBlock.Body().BlobKzgCommitments()
	if err != nil {
		return nil, &core.RpcError{Err: errors.Wrapf(err, "failed to retrieve kzg commitments from block %#x", rootSlice), Reason: core.Internal}
	}

	// If there are no commitments return 200 w/ empty list
	if len(commitments) == 0 {
		return make([]*blocks.VerifiedROBlob, 0), nil
	}

	// Compute the first Fulu slot.
	fuluForkEpoch := params.BeaconConfig().FuluForkEpoch
	fuluForkSlot := primitives.Slot(math.MaxUint64)
	if fuluForkEpoch != primitives.Epoch(math.MaxUint64) {
		fuluForkSlot, err = slots.EpochStart(fuluForkEpoch)
		if err != nil {
			return nil, &core.RpcError{Err: errors.Wrap(err, "could not calculate peerDAS start slot"), Reason: core.Internal}
		}
	}

	// Convert versioned hashes to indices if provided
	indices := cfg.Indices
	if len(cfg.VersionedHashes) > 0 {
		// Build a map of requested versioned hashes for fast lookup and tracking
		requestedHashes := make(map[string]bool)
		for _, versionedHash := range cfg.VersionedHashes {
			requestedHashes[string(versionedHash)] = true
		}

		// Create indices array and track which hashes we found
		indices = make([]int, 0, len(cfg.VersionedHashes))
		foundHashes := make(map[string]bool)

		for i, commitment := range commitments {
			versionedHash := primitives.ConvertKzgCommitmentToVersionedHash(commitment)
			hashStr := string(versionedHash[:])
			if requestedHashes[hashStr] {
				indices = append(indices, i)
				foundHashes[hashStr] = true
			}
		}

		// Check if all requested hashes were found
		if len(indices) != len(cfg.VersionedHashes) {
			// Collect missing hashes
			missingHashes := make([]string, 0, len(cfg.VersionedHashes)-len(indices))
			for _, requestedHash := range cfg.VersionedHashes {
				if !foundHashes[string(requestedHash)] {
					missingHashes = append(missingHashes, hexutil.Encode(requestedHash))
				}
			}

			// Create detailed error message
			errMsg := fmt.Sprintf("versioned hash(es) not found in block (requested %d hashes, found %d, missing: %v)",
				len(cfg.VersionedHashes), len(indices), missingHashes)

			return nil, &core.RpcError{Err: errors.New(errMsg), Reason: core.NotFound}
		}
	}

	if roBlock.Slot() >= fuluForkSlot {
		roBlock, err := blocks.NewROBlockWithRoot(roSignedBlock, root)
		if err != nil {
			return nil, &core.RpcError{Err: errors.Wrapf(err, "failed to create roBlock with root %#x", root), Reason: core.Internal}
		}

		return p.blobsFromStoredDataColumns(roBlock, indices)
	}

	return p.blobsFromStoredBlobs(commitments, root, indices)
}

// blobsFromStoredBlobs retrieves blob sidercars corresponding to `indices` and `root` from the store.
// This function expects blob sidecars to be stored (aka. no data column sidecars).
func (p *BeaconDbBlocker) blobsFromStoredBlobs(commitments [][]byte, root [fieldparams.RootLength]byte, indices []int) ([]*blocks.VerifiedROBlob, *core.RpcError) {
	summary := p.BlobStorage.Summary(root)
	maxBlobCount := summary.MaxBlobsForEpoch()

	for _, index := range indices {
		if uint64(index) >= maxBlobCount {
			return nil, &core.RpcError{
				Err:    fmt.Errorf("requested index %d is bigger than the maximum possible blob count %d", index, maxBlobCount),
				Reason: core.BadRequest,
			}
		}

		if !summary.HasIndex(uint64(index)) {
			return nil, &core.RpcError{
				Err:    fmt.Errorf("requested index %d not found", index),
				Reason: core.NotFound,
			}
		}
	}

	// If no indices are provided, use all indices that are available in the summary.
	if len(indices) == 0 {
		for index := range commitments {
			if summary.HasIndex(uint64(index)) {
				indices = append(indices, index)
			}
		}
	}

	// Retrieve blob sidecars from the store.
	blobs := make([]*blocks.VerifiedROBlob, 0, len(indices))
	for _, index := range indices {
		blobSidecar, err := p.BlobStorage.Get(root, uint64(index))
		if err != nil {
			return nil, &core.RpcError{
				Err:    fmt.Errorf("could not retrieve blob for block root %#x at index %d", root, index),
				Reason: core.Internal,
			}
		}

		blobs = append(blobs, &blobSidecar)
	}

	return blobs, nil
}

// blobsFromStoredDataColumns retrieves data column sidecars from the store,
// reconstructs the whole matrix if needed, converts the matrix to blobs,
// and then returns converted blobs corresponding to `indices` and `root`.
// This function expects data column sidecars to be stored (aka. no blob sidecars).
// If not enough data column sidecars are available to convert blobs from them
// (either directly or after reconstruction), an error is returned.
func (p *BeaconDbBlocker) blobsFromStoredDataColumns(block blocks.ROBlock, indices []int) ([]*blocks.VerifiedROBlob, *core.RpcError) {
	root := block.Root()

	// Use all indices if none are provided.
	if len(indices) == 0 {
		commitments, err := block.Block().Body().BlobKzgCommitments()
		if err != nil {
			return nil, &core.RpcError{
				Err:    errors.Wrap(err, "could not retrieve blob commitments"),
				Reason: core.Internal,
			}
		}

		for index := range commitments {
			indices = append(indices, index)
		}
	}

	// Count how many columns we have in the store.
	summary := p.DataColumnStorage.Summary(root)
	stored := summary.Stored()
	count := uint64(len(stored))

	if count < peerdas.MinimumColumnCountToReconstruct() {
		// There is no way to reconstruct the data columns.
		return nil, &core.RpcError{
			Err:    errors.Errorf("the node does not custody enough data columns to reconstruct blobs - please start the beacon node with the `--%s` flag to ensure this call to succeed, or retry later if it is already the case", flags.SubscribeAllDataSubnets.Name),
			Reason: core.NotFound,
		}
	}

	// Retrieve from the database needed data columns.
	verifiedRoDataColumnSidecars, err := p.neededDataColumnSidecars(root, stored)
	if err != nil {
		return nil, &core.RpcError{
			Err:    errors.Wrap(err, "needed data column sidecars"),
			Reason: core.Internal,
		}
	}

	// Reconstruct blob sidecars from data column sidecars.
	verifiedRoBlobSidecars, err := peerdas.ReconstructBlobs(block, verifiedRoDataColumnSidecars, indices)
	if err != nil {
		return nil, &core.RpcError{
			Err:    errors.Wrap(err, "blobs from data columns"),
			Reason: core.Internal,
		}
	}

	return verifiedRoBlobSidecars, nil
}

// neededDataColumnSidecars retrieves all data column sidecars corresponding to (non extended) blobs if available,
// else retrieves all data column sidecars from the store.
func (p *BeaconDbBlocker) neededDataColumnSidecars(root [fieldparams.RootLength]byte, stored map[uint64]bool) ([]blocks.VerifiedRODataColumn, error) {
	// Check if we have all the non-extended data columns.
	cellsPerBlob := fieldparams.CellsPerBlob
	blobIndices := make([]uint64, 0, cellsPerBlob)
	hasAllBlobColumns := true
	for i := range uint64(cellsPerBlob) {
		if !stored[i] {
			hasAllBlobColumns = false
			break
		}
		blobIndices = append(blobIndices, i)
	}

	if hasAllBlobColumns {
		// Retrieve only the non-extended data columns.
		verifiedRoSidecars, err := p.DataColumnStorage.Get(root, blobIndices)
		if err != nil {
			return nil, errors.Wrap(err, "data columns storage get")
		}

		return verifiedRoSidecars, nil
	}

	// Retrieve all the data columns.
	verifiedRoSidecars, err := p.DataColumnStorage.Get(root, nil)
	if err != nil {
		return nil, errors.Wrap(err, "data columns storage get")
	}

	return verifiedRoSidecars, nil
}
