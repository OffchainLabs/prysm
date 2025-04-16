package lookup

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

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
	Blobs(ctx context.Context, id string, indices map[uint64]bool) ([]*blocks.VerifiedROBlob, *core.RpcError)
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

// uint64MapToSortedSlice produces a sorted uint64 slice from a map.
func uint64MapToSortedSlice(input map[uint64]bool) []uint64 {
	output := make([]uint64, 0, len(input))
	for idx := range input {
		output = append(output, idx)
	}
	slices.Sort[[]uint64](output)
	return output
}

// blobsFromStoredBlobs retrieves blobs corresponding to `indices` and `root` from the store.
// This function expects blobs to be stored directly (aka. no data columns).
func (p *BeaconDbBlocker) blobsFromStoredBlobs(
	indices map[uint64]bool,
	root []byte,
	slot primitives.Slot,
	commitments [][]byte,
) ([]*blocks.VerifiedROBlob, *core.RpcError) {
	if len(indices) == 0 {
		sum := p.BlobStorage.Summary(bytesutil.ToBytes32(root))
		for i := range commitments {
			if sum.HasIndex(uint64(i)) {
				indices[uint64(i)] = true
			}
		}
	}

	// Sort the indices to ensure the blobs are returned in the correct order.
	// This is not specifically required by the spec, but it makes the response deterministic.
	sortedIndices := uint64MapToSortedSlice(indices)

	// returns empty slice if there are no indices
	blobs := make([]*blocks.VerifiedROBlob, 0, len(indices))
	for _, index := range sortedIndices {
		vblob, err := p.BlobStorage.Get(bytesutil.ToBytes32(root), index)
		if err != nil {
			log.WithFields(log.Fields{
				"blockRoot": hexutil.Encode(root),
				"blobIndex": index,
			}).Error(errors.Wrapf(err, "could not retrieve blob for block root %#x at index %d", root, index))
			return nil, &core.RpcError{Err: fmt.Errorf("could not retrieve blob for block root %#x at index %d", root, index), Reason: core.Internal}
		}
		blobs = append(blobs, &vblob)
	}
	return blobs, nil
}

// blobsFromNonExtendedStoredDataColumns load the non-extended data columns from the store corresponding to `root` and returns the verified RO blobs.
// This function assumes that all the non-extended data columns are available in the store.
func (p *BeaconDbBlocker) blobsFromNonExtendedStoredDataColumns(
	root [fieldparams.RootLength]byte,
	blobIndices map[uint64]bool,
) ([]*blocks.VerifiedROBlob, *core.RpcError) {
	nonExtendedColumnsCount := uint64(fieldparams.NumberOfColumns / 2)

	// Load the data columns corresponding to the non-extended blobs.
	dataColumnIndices := make([]uint64, 0, nonExtendedColumnsCount)
	for index := range nonExtendedColumnsCount {
		dataColumnIndices = append(dataColumnIndices, index)
	}

	verifiedRODataColumns, err := p.DataColumnStorage.Get(root, dataColumnIndices)
	if err != nil {
		log.WithField("blockRoot", hexutil.Encode(root[:])).Error(errors.Wrapf(err, "missing data columns for block root %#x", root))

		return nil, &core.RpcError{
			Err:    fmt.Errorf("could not retrieve data column sidecars for block root %#x", root),
			Reason: core.Internal,
		}
	}

	if uint64(len(verifiedRODataColumns)) != nonExtendedColumnsCount {
		log.WithField("blockRoot", hexutil.Encode(root[:])).Error("not enough data columns returned to extract blobs")

		return nil, &core.RpcError{
			Err:    fmt.Errorf("not enough data columns returned to extract blobs for block root %#x", root),
			Reason: core.Internal,
		}
	}

	storedDataColumnsSidecar := make([]*ethpb.DataColumnSidecar, 0, nonExtendedColumnsCount)
	for _, verifiedRODataColumn := range verifiedRODataColumns {
		storedDataColumnsSidecar = append(storedDataColumnsSidecar, verifiedRODataColumn.DataColumnSidecar)
	}

	// Get verified RO blobs from the data columns.
	verifiedROBlobs, err := peerdas.Blobs(blobIndices, storedDataColumnsSidecar)
	if err != nil {
		log.WithField("blockRoot", hexutil.Encode(root[:])).Error(errors.Wrap(err, "could not compute blobs from data columns"))
		return nil, &core.RpcError{Err: errors.Wrap(err, "could not compute blobs from data columns"), Reason: core.Internal}
	}

	return verifiedROBlobs, nil
}

// blobsFromReconstructedDataColumns retrieves data columns from the store, reconstruct the whole matrix and returns the verified RO blobs.
func (p *BeaconDbBlocker) blobsFromReconstructedDataColumns(
	root [fieldparams.RootLength]byte,
	blobIndices map[uint64]bool,
) ([]*blocks.VerifiedROBlob, *core.RpcError) {
	// Load all possible data columns for this block root.
	// Theoretically, we could only retrieve the minimum number of columns needed to reconstruct the missing ones,
	// but here we make the assumption that the cost of loading all the columns from the store is negligible
	// compared to the cost of reconstructing them.
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	verifiedRODataColumnSidecars, err := p.DataColumnStorage.Get(root, nil)
	if err != nil {
		log.WithFields(log.Fields{
			"blockRoot": hexutil.Encode(root[:]),
		}).Error(errors.Wrapf(err, "get data column sidecars"))

		return nil, &core.RpcError{
			Err:    fmt.Errorf("could not retrieve data column sidecars for block root %#x", root),
			Reason: core.Internal,
		}
	}

	if uint64(len(verifiedRODataColumnSidecars)) < numberOfColumns/2 {
		log.WithField("blockRoot", hexutil.Encode(root[:])).Error("not enough data columns returned to reconstruct blobs")

		return nil, &core.RpcError{
			Err:    fmt.Errorf("not enough data columns returned to reconstruct blobs for block root %#x", root),
			Reason: core.Internal,
		}
	}

	storedDataColumnsSidecar := make([]*ethpb.DataColumnSidecar, 0, len(verifiedRODataColumnSidecars))
	for _, verifiedRODataColumn := range verifiedRODataColumnSidecars {
		storedDataColumnsSidecar = append(storedDataColumnsSidecar, verifiedRODataColumn.DataColumnSidecar)
	}

	// Recover cells and proofs.
	recoveredCellsAndProofs, err := peerdas.RecoverCellsAndProofs(storedDataColumnsSidecar, root)
	if err != nil {
		log.WithField("blockRoot", hexutil.Encode(root[:])).Error(errors.Wrap(err, "could not recover cells and proofs"))
		return nil, &core.RpcError{Err: errors.Wrap(err, "could not recover cells and proofs"), Reason: core.Internal}
	}

	// It is safe to retrieve the first element, since we already know there is at least one column in the store.
	firstDataColumnSidecar := storedDataColumnsSidecar[0]

	// Reconstruct the data columns sidecars.
	reconstructedDataColumnSidecars, err := peerdas.DataColumnSidecarsForReconstruct(
		firstDataColumnSidecar.KzgCommitments,
		firstDataColumnSidecar.SignedBlockHeader,
		firstDataColumnSidecar.KzgCommitmentsInclusionProof,
		recoveredCellsAndProofs,
	)
	if err != nil {
		log.WithField("blockRoot", hexutil.Encode(root[:])).Error(errors.Wrap(err, "could not reconstruct data columns sidecars"))
		return nil, &core.RpcError{Err: errors.Wrap(err, "could not reconstruct data columns sidecars"), Reason: core.Internal}
	}

	// Get verified RO blobs from the data columns.
	verifiedROBlobs, err := peerdas.Blobs(blobIndices, reconstructedDataColumnSidecars)
	if err != nil {
		log.WithField("blockRoot", hexutil.Encode(root[:])).Error(errors.Wrap(err, "could not compute blobs from data columns"))
		return nil, &core.RpcError{Err: errors.Wrap(err, "could not compute blobs from data columns"), Reason: core.Internal}
	}

	return verifiedROBlobs, nil
}

// blobsFromStoredDataColumns retrieves data columns from the store, reconstruct the whole matrix if needed, convert the matrix to blobs,
// and then returns blobs corresponding to `indices` and `root` from the store,
// This function expects data columns to be stored (aka. no blobs).
// If not enough data columns are available to extract blobs from them (either directly or after reconstruction), an error is returned.
func (p *BeaconDbBlocker) blobsFromStoredDataColumns(indices map[uint64]bool, rootBytes []byte) ([]*blocks.VerifiedROBlob, *core.RpcError) {
	beaconConfig := params.BeaconConfig()
	numberOfColumns := beaconConfig.NumberOfColumns
	numberOfCustodyGroups := beaconConfig.NumberOfCustodyGroups
	columnsPerGroup := numberOfColumns / numberOfCustodyGroups

	root := bytesutil.ToBytes32(rootBytes)

	// Retrieve the data columns indice actually we store.
	summary := p.DataColumnStorage.Summary(root)

	storedDataColumnsIndices := make(map[uint64]bool, numberOfColumns)
	for i := range numberOfColumns {
		if summary.HasIndex(i) {
			storedDataColumnsIndices[i] = true
		}
	}

	storedDataColumnCount := uint64(len(storedDataColumnsIndices))
	storedGroupCount := storedDataColumnCount / columnsPerGroup

	// Determine is we are able to reconstruct the data columns.
	canReconstruct := peerdas.CanSelfReconstruct(storedGroupCount)

	if !canReconstruct {
		// There is no way to reconstruct the data columns.
		return nil, &core.RpcError{
			Err:    errors.Errorf("the node does not custody enough data columns to reconstruct blobs. Please start the beacon node with the `--%s` flag to ensure this call to success, or retry later if it already the case.", flags.SubscribeToAllSubnets.Name),
			Reason: core.NotFound,
		}
	}

	nonExtendedColumnsCount := uint64(fieldparams.NumberOfColumns / 2)

	// Check if we store all the non extended columns. If so, we can respond without reconstructing.
	missingColumns := make(map[uint64]bool)
	for columnIndex := range nonExtendedColumnsCount {
		if _, ok := storedDataColumnsIndices[columnIndex]; !ok {
			missingColumns[columnIndex] = true
		}
	}

	if len(missingColumns) == 0 {
		// No need to reconstruct, this is the happy path.
		return p.blobsFromNonExtendedStoredDataColumns(root, indices)
	}

	// Some non-extended data columns are missing, we need to reconstruct them.
	return p.blobsFromReconstructedDataColumns(root, indices)
}

// Blobs returns the blobs for a given block id identifier and blob indices. The identifier can be one of:
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
//   - block exists, no commitment, 200 w/ empty list
//   - block exists, has commitments, inside retention period (greater of protocol- or user-specified) serve then w/ 200 unless we hit an error reading them.
//     we are technically not supposed to import a block to forkchoice unless we have the blobs, so the nuance here is if we can't find the file and we are inside the protocol-defined retention period, then it's actually a 500.
//   - block exists, has commitments, outside retention period (greater of protocol- or user-specified) - ie just like block exists, no commitment
func (p *BeaconDbBlocker) Blobs(ctx context.Context, id string, indices map[uint64]bool) ([]*blocks.VerifiedROBlob, *core.RpcError) {
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

	b, err := p.BeaconDB.Block(ctx, root)
	if err != nil {
		return nil, &core.RpcError{Err: errors.Wrapf(err, "failed to retrieve block %#x from db", rootSlice), Reason: core.Internal}
	}
	if b == nil {
		return nil, &core.RpcError{Err: fmt.Errorf("block %#x not found in db", rootSlice), Reason: core.NotFound}
	}

	// if block is not in the retention window, return 200 w/ empty list
	if !p.BlobStorage.WithinRetentionPeriod(slots.ToEpoch(b.Block().Slot()), slots.ToEpoch(p.GenesisTimeFetcher.CurrentSlot())) {
		return make([]*blocks.VerifiedROBlob, 0), nil
	}

	commitments, err := b.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, &core.RpcError{Err: errors.Wrapf(err, "failed to retrieve kzg commitments from block %#x", rootSlice), Reason: core.Internal}
	}
	// if there are no commitments return 200 w/ empty list
	if len(commitments) == 0 {
		return make([]*blocks.VerifiedROBlob, 0), nil
	}

	if indices == nil {
		indices = make(map[uint64]bool)
	}

	// Get the slot of the block.
	blockSlot := b.Block().Slot()

	// Get the first peerDAS epoch.
	fuluForkEpoch := params.BeaconConfig().FuluForkEpoch

	// Compute the first peerDAS slot.
	peerDASStartSlot := primitives.Slot(math.MaxUint64)
	if fuluForkEpoch != primitives.Epoch(math.MaxUint64) {
		peerDASStartSlot, err = slots.EpochStart(fuluForkEpoch)
		if err != nil {
			return nil, &core.RpcError{Err: errors.Wrap(err, "could not calculate peerDAS start slot"), Reason: core.Internal}
		}
	}

	// Is peerDAS enabled for this block?
	isPeerDASEnabledForBlock := blockSlot >= peerDASStartSlot

	if isPeerDASEnabledForBlock {
		return p.blobsFromStoredDataColumns(indices, rootSlice)
	}

	return p.blobsFromStoredBlobs(indices, rootSlice, blockSlot, commitments)
}
