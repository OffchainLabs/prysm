package sync

import (
	"context"
	goErrors "errors"
	"fmt"
	"sort"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync/verify"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	leakybucket "github.com/OffchainLabs/prysm/v6/container/leaky-bucket"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	ErrNotEnoughColsAvailable = errors.New("not enough columns available across peers for reconstruction")
	ErrReconstructionFailed   = errors.New("failed to reconstruct data columns")
)

type UnavailableColumnsError struct {
	Columns []uint64
}

var _ error = &UnavailableColumnsError{}

func (e *UnavailableColumnsError) Error() string {
	return fmt.Sprintf("no peers available to fetch data columns from: %v", e.Columns)
}

func (e *UnavailableColumnsError) Is(target error) bool {
	_, ok := target.(*UnavailableColumnsError)
	return ok
}

func NewUnavailableColumnsError(columns []uint64) *UnavailableColumnsError {
	return &UnavailableColumnsError{Columns: columns}
}

// RequestDataColumnSidecarsByRoot is an opinionated, high level function which, for each data column in `dataColumnsToFetch`:
//   - Greedily selects, among `peers`, the peers that can provide the requested data columns, to minimize the number of requests.
//   - Request the data column sidecars from the selected peers.
//   - In case of peers unable to actually provide all the requested data columns, retry with other peers.
//
// This function:
//   - returns on success when all the initially missing sidecars in `dataColumnsToFetch` are retrieved, or
//   - returns an error if all peers in `peers` are exhausted and at least one data column sidecar is still missing.
//
// TODO: In case at least one column is still missing after peer exhaustion,
//
//	but `peers` custody more than 64 columns, then try to fetch enough columns to reconstruct needed ones.
func RequestDataColumnSidecarsByRoot(
	ctx context.Context,
	dataColumnsToFetch []uint64,
	block blocks.ROBlock,
	peers []core.PeerID,
	clock *startup.Clock,
	p2p p2p.P2P,
	ctxMap ContextByteVersions,
	newColumnsVerifier verification.NewDataColumnsVerifier,
) ([]blocks.VerifiedRODataColumn, error) {
	if len(dataColumnsToFetch) == 0 {
		return nil, nil
	}

	// Assemble the peers who can provide the needed data columns.
	dataColumnsByAdmissiblePeer, _, _, err := AdmissiblePeersForDataColumns(peers, dataColumnsToFetch, p2p)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't get admissible peers for data columns")
	}

	// If the request was non-empty but no peers were found for any needed column,
	// return the specific error immediately.
	if len(dataColumnsToFetch) > 0 && len(dataColumnsByAdmissiblePeer) == 0 {
		// No peer has any of the requested columns.
		return nil, NewUnavailableColumnsError(dataColumnsToFetch)
	}

	verifiedSidecars := make([]blocks.VerifiedRODataColumn, 0, len(dataColumnsToFetch))
	remainingMissingColumns := make(map[uint64]bool, len(dataColumnsToFetch))
	for _, column := range dataColumnsToFetch {
		remainingMissingColumns[column] = true
	}

	blockRoot := block.Root()

	for len(dataColumnsByAdmissiblePeer) > 0 {
		peersToFetchFrom, err := SelectPeersToFetchDataColumnsFrom(uint64MapToSortedSlice(remainingMissingColumns), dataColumnsByAdmissiblePeer)
		if err != nil {
			return nil, errors.Wrap(err, "select peers to fetch data columns from")
		}

		// Request the data columns from each peer.
		successfulColumns := make(map[uint64]bool, len(remainingMissingColumns))
		for peer, peerRequestedColumns := range peersToFetchFrom {
			log := log.WithFields(logrus.Fields{"peer": peer.String(), "blockRoot": fmt.Sprintf("%#x", blockRoot)})

			// Build the requests for the data columns.
			byRootRequests := make(types.DataColumnSidecarsByRootReq, 0, len(peerRequestedColumns))
			for _, column := range peerRequestedColumns {
				byRootRequest := &eth.DataColumnIdentifier{BlockRoot: blockRoot[:], Index: column}
				byRootRequests = append(byRootRequests, byRootRequest)
			}

			// Send the requests to the peer.
			peerSidecars, err := SendDataColumnSidecarsByRootRequest(ctx, clock, p2p, peer, ctxMap, &byRootRequests)
			if err != nil {
				// Remove this peer since it failed to respond correctly.
				delete(dataColumnsByAdmissiblePeer, peer)

				log.WithFields(logrus.Fields{
					"peer":      peer.String(),
					"blockRoot": fmt.Sprintf("%#x", block.Root()),
				}).WithError(err).Debug("Failed to request data columns from peer")

				continue
			}

			verifiedPeerSidecars, err := verifyColumnsForBlock(block, peerSidecars, newColumnsVerifier)
			if err != nil {
				// Remove this peer since it failed to respond correctly.
				delete(dataColumnsByAdmissiblePeer, peer)
				log.WithError(err).Debug("Failed to verify columns for block")
				return nil, errors.Wrap(err, "verify columns for block")
			}

			// Mark columns as successful
			for _, sidecar := range verifiedPeerSidecars {
				successfulColumns[sidecar.Index] = true
			}

			// Check if all requested columns were successfully returned.
			peerMissingColumns := make(map[uint64]bool)
			for _, index := range peerRequestedColumns {
				if !successfulColumns[index] {
					peerMissingColumns[index] = true
				}
			}

			if len(peerMissingColumns) > 0 {
				// Remove this peer if some requested columns were not correctly returned.
				delete(dataColumnsByAdmissiblePeer, peer)
				log.WithField("missingColumns", uint64MapToSortedSlice(peerMissingColumns)).Debug("Peer did not provide all requested data columns")
			}

			verifiedSidecars = append(verifiedSidecars, verifiedPeerSidecars...)
		}

		// Update remaining columns for the next retry.
		for col := range successfulColumns {
			delete(remainingMissingColumns, col)
		}

		if len(remainingMissingColumns) > 0 {
			// Some columns are still missing, retry with the remaining peers.
			continue
		}

		return verifiedSidecars, nil
	}

	// If we still have remaining columns after all retries, return error
	return nil, fmt.Errorf("failed to retrieve all requested data columns after retries for block root=%#x, %w", blockRoot, NewUnavailableColumnsError(uint64MapToSortedSlice(remainingMissingColumns)))
}

func verifyColumnsForBlock(block blocks.ROBlock, columns []blocks.RODataColumn, newColumnsVerifier verification.NewDataColumnsVerifier) ([]blocks.VerifiedRODataColumn, error) {
	// Check if returned data columns align with the block.
	if err := verify.DataColumnsAlignWithBlock(block, columns); err != nil {
		return nil, errors.Wrap(err, "align with block failed")
	}

	// Verify the received sidecars.
	verifier := newColumnsVerifier(columns, verification.ByRootRequestDataColumnSidecarRequirements)

	if err := verifier.Valid(); err != nil {
		return nil, errors.Wrap(err, "valid verification failed")
	}

	if err := verifier.SidecarInclusionProven(); err != nil {
		return nil, errors.Wrap(err, "sidecar inclusion proof verification failed")
	}

	if err := verifier.SidecarKzgProofVerified(); err != nil {
		return nil, errors.Wrap(err, "sidecar KZG proof verification failed")
	}

	// Upgrade the sidecars to verified sidecars.
	verifiedColumns, err := verifier.VerifiedRODataColumns()
	if err != nil {
		// This should never happen.
		return nil, errors.Wrap(err, "verified data columns")
	}

	return verifiedColumns, nil
}

// ReconstructDataColumnsByRoot attempts to reconstruct a requested set of data columns for a given block.
// It identifies available columns from peers, calculates the recovery threshold, fetches a set of 'recoveryThreshold' columns.
// Once a fetch succeeds, it uses erasure coding to reconstruct the full set of columns and returns
// the originally requested ones. Errors during fetch or reconstruction are fatal.
func ReconstructDataColumnsByRoot(
	ctx context.Context,
	dataColumnsToFetch []uint64,
	block blocks.ROBlock,
	peers []core.PeerID,
	clock *startup.Clock,
	p2p p2p.P2P,
	ctxMap ContextByteVersions,
	newColumnsVerifier verification.NewDataColumnsVerifier,
) ([]blocks.VerifiedRODataColumn, error) {
	if len(dataColumnsToFetch) == 0 {
		return nil, nil // Nothing requested, nothing to recover.
	}

	// Determine recovery threshold.
	numberOfColumns := int(params.BeaconConfig().NumberOfColumns)
	// If total is odd, then we need total / 2 + 1 columns to reconstruct.
	// If total is even, then we need total / 2 columns to reconstruct.
	recoveryThreshold := numberOfColumns/2 + numberOfColumns%2
	log := log.WithFields(logrus.Fields{
		"blockRoot":         fmt.Sprintf("%#x", block.Root()),
		"totalColumns":      numberOfColumns,
		"recoveryThreshold": recoveryThreshold,
		"requestedColumns":  dataColumnsToFetch,
	})
	log.Debug("Attempting data column recovery")

	// Find available columns from peers.
	allCols := make([]uint64, numberOfColumns)
	for i := 0; i < numberOfColumns; i++ {
		allCols[i] = uint64(i)
	}
	_, admissiblePeersByDataColumn, _, err := AdmissiblePeersForDataColumns(peers, allCols, p2p)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't get admissible peers for all data columns")
	}

	dataColumnsAvailable := make([]uint64, len(admissiblePeersByDataColumn))
	for colIdx := range admissiblePeersByDataColumn {
		dataColumnsAvailable = append(dataColumnsAvailable, colIdx)
	}

	if len(dataColumnsAvailable) < recoveryThreshold {
		return nil, ErrNotEnoughColsAvailable
	}

	log.WithFields(logrus.Fields{
		"dataColumnsAvailable": dataColumnsAvailable,
		"numPeers":             len(peers),
	}).Debug("Available columns for reconstruction")

	// Fetch the required sidecars for reconstruction.
	fetchedSidecars, err := fetchAndVerifyRecoveryColumns(
		ctx, dataColumnsToFetch, dataColumnsAvailable, recoveryThreshold,
		block, peers, clock, p2p, ctxMap, newColumnsVerifier,
	)
	if err != nil {
		return nil, err
	}

	// Perform reconstruction and filter for originally requested columns.
	return reconstructAndFilterColumns(dataColumnsToFetch, fetchedSidecars, block, newColumnsVerifier)
}

// FetchOrReconstructDataColumnsByRoot attempts to fetch the requested data columns from peers.
// If fetching fails specifically because no peers are available (UnavailableColumnsError),
// it attempts to reconstruct the columns using available data from other peers.
func FetchOrReconstructDataColumnsByRoot(
	ctx context.Context,
	dataColumnsToFetch []uint64,
	block blocks.ROBlock,
	peers []core.PeerID,
	clock *startup.Clock,
	p2p p2p.P2P,
	ctxMap ContextByteVersions,
	newColumnsVerifier verification.NewDataColumnsVerifier,
) ([]blocks.VerifiedRODataColumn, error) {
	log := log.WithFields(logrus.Fields{
		"blockRoot":          fmt.Sprintf("%#x", block.Root()),
		"dataColumnsToFetch": dataColumnsToFetch,
	})
	log.Debug("Attempting to fetch or reconstruct data columns")

	// First, try to request the columns directly.
	sidecars, err := RequestDataColumnSidecarsByRoot(
		ctx,
		dataColumnsToFetch,
		block,
		peers,
		clock,
		p2p,
		ctxMap,
		newColumnsVerifier,
	)
	if err == nil {
		// Successfully fetched.
		log.Debug("Successfully fetched requested data columns")
		return sidecars, nil
	}

	// If the error is specifically UnavailableColumnsError, attempt reconstruction.
	if errors.Is(err, &UnavailableColumnsError{}) {
		log.WithError(err).Debug("Fetching failed due to no available peers, attempting reconstruction")
		reconstructedSidecars, reconstructErr := ReconstructDataColumnsByRoot(
			ctx,
			dataColumnsToFetch,
			block,
			peers,
			clock,
			p2p,
			ctxMap,
			newColumnsVerifier,
		)
		if reconstructErr != nil {
			joinedErr := goErrors.Join(err, reconstructErr)
			return nil, errors.Wrap(joinedErr, "failed to fetch (no peers) and reconstruction failed")
		}
		// Successfully reconstructed.
		log.Debug("Successfully reconstructed requested data columns")
		return reconstructedSidecars, nil
	}

	// If fetching failed for a different reason, return that error directly.
	return nil, errors.Wrap(err, "failed to fetch data columns")
}

// fetchAndVerifyRecoveryColumns selects a prioritized set of recoveryThreshold columns to fetch,
// requests them using RequestDataColumnSidecarsByRoot, and verifies enough were received.
func fetchAndVerifyRecoveryColumns(
	ctx context.Context,
	dataColumnsToFetch []uint64,
	dataColumnsAvailable []uint64,
	recoveryThreshold int,
	block blocks.ROBlock,
	peers []core.PeerID,
	clock *startup.Clock,
	p2p p2p.P2P,
	ctxMap ContextByteVersions,
	newColumnsVerifier verification.NewDataColumnsVerifier,
) ([]blocks.VerifiedRODataColumn, error) {
	// Select columns to fetch using the helper function.
	columnsToFetch, err := selectRecoveryColumnsToFetch(dataColumnsToFetch, dataColumnsAvailable, recoveryThreshold)
	if err != nil {
		return nil, err // Propagate the error if selection failed.
	}

	log.WithFields(logrus.Fields{
		"columnsToFetch":    columnsToFetch,
		"recoveryThreshold": recoveryThreshold,
	}).Debug("Selected columns for fetch for recovery")

	// If we encounter advertised columns that are not available, we filter them during each iteration.
	filteredAvailableColumns := make(map[uint64]bool, len(dataColumnsAvailable))
	for _, col := range dataColumnsAvailable {
		filteredAvailableColumns[col] = true
	}

	var fetchedSidecars []blocks.VerifiedRODataColumn
	for {
		// Fetch selected columns.
		fetchedSidecars, err = RequestDataColumnSidecarsByRoot(
			ctx,
			columnsToFetch,
			block,
			peers,
			clock,
			p2p,
			ctxMap,
			newColumnsVerifier,
		)
		if err == nil {
			// Successfully fetched data columns.
			break
		}

		ucErr := &UnavailableColumnsError{}
		if errors.As(err, &ucErr) {
			// If some of the columns for reconstruction are unavailable, try again with those columns removed from the available columns.
			for _, unavailableCol := range ucErr.Columns {
				delete(filteredAvailableColumns, unavailableCol)
			}
			localAvailableColumns := make([]uint64, 0, len(filteredAvailableColumns))
			for col := range filteredAvailableColumns {
				localAvailableColumns = append(localAvailableColumns, col)
			}

			columnsToFetch, err = selectRecoveryColumnsToFetch(dataColumnsToFetch, localAvailableColumns, recoveryThreshold)
			if err != nil {
				return nil, errors.Wrap(err, "failed to select columns for recovery")
			}
		} else {
			// If the error is not an UnavailableColumnsError, return the error directly.
			return nil, errors.Wrap(err, "failed to request data columns for recovery")
		}
	}

	// Check if we actually got enough sidecars after the request.
	if len(fetchedSidecars) < recoveryThreshold {
		return nil, errors.Wrapf(ErrReconstructionFailed, "received only %d columns, need %d", len(fetchedSidecars), recoveryThreshold)
	}

	log.WithField("fetchedCount", len(fetchedSidecars)).Debug("Successfully fetched required data columns")
	return fetchedSidecars, nil
}

// selectRecoveryColumnsToFetch determines the set of columns to fetch for recovery.
// It prioritizes columns that were originally requested and are available,
// then fills the remaining slots up to the recoveryThreshold with other available columns,
// sorted by index.
func selectRecoveryColumnsToFetch(
	dataColumnsToFetch []uint64,
	dataColumnsAvailable []uint64,
	recoveryThreshold int,
) ([]uint64, error) {
	if len(dataColumnsAvailable) < recoveryThreshold {
		return nil, ErrNotEnoughColsAvailable
	}

	columnsToFetch := make([]uint64, 0, recoveryThreshold)
	isColumnAvailable := make(map[uint64]bool, len(dataColumnsAvailable))
	for _, col := range dataColumnsAvailable {
		isColumnAvailable[col] = true
	}

	// Add requested and available columns first.
	usedColumns := make(map[uint64]bool, len(dataColumnsToFetch))
	for _, col := range dataColumnsToFetch {
		if isColumnAvailable[col] {
			columnsToFetch = append(columnsToFetch, col)
			usedColumns[col] = true
		}
	}

	// Prepare a sorted list of other available columns not yet selected.
	otherAvailable := make([]uint64, 0, len(dataColumnsAvailable))
	for _, col := range dataColumnsAvailable {
		if !usedColumns[col] { // If not already added
			otherAvailable = append(otherAvailable, col)
			usedColumns[col] = true
		}
	}
	sort.Slice(otherAvailable, func(i, j int) bool { return otherAvailable[i] < otherAvailable[j] })

	// Fill up to the recovery threshold.
	columnsToFetch = append(columnsToFetch, otherAvailable[:recoveryThreshold-len(columnsToFetch)]...)

	return columnsToFetch, nil
}

// reconstructAndFilterColumns takes successfully fetched sidecars and performs the erasure coding
// reconstruction process. It then filters the results to return only the columns
// originally requested.
func reconstructAndFilterColumns(
	dataColumnsToFetch []uint64,
	fetchedSidecars []blocks.VerifiedRODataColumn,
	block blocks.ROBlock,
	newColumnsVerifier verification.NewDataColumnsVerifier,
) ([]blocks.VerifiedRODataColumn, error) {
	// Convert fetched VerifiedRODataColumn to []*ethpb.DataColumnSidecar for peerdas functions.
	pbFetchedSidecars := make([]*ethpb.DataColumnSidecar, len(fetchedSidecars))
	for i := range fetchedSidecars {
		pbFetchedSidecars[i] = fetchedSidecars[i].DataColumnSidecar
	}

	recoveredCellsAndProofs, err := peerdas.RecoverCellsAndProofs(pbFetchedSidecars, block.Root())
	if err != nil {
		log.WithError(err).Error("Failed to recover cells and proofs after fetching columns")
		return nil, errors.Wrapf(ErrReconstructionFailed, "peerdas.RecoverCellsAndProofs failed: %v", err)
	}

	blockBody := block.Block().Body()
	blobKzgCommitments, err := blockBody.BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "could not get blob KZG commitments from block body")
	}
	signedBlockHeader, err := block.Header()
	if err != nil {
		return nil, errors.Wrap(err, "could not get signed block header")
	}
	kzgCommitmentsInclusionProof, err := blocks.MerkleProofKZGCommitments(blockBody)
	if err != nil {
		return nil, errors.Wrap(err, "could not get KZG commitments inclusion proof")
	}

	pbSidecars, err := peerdas.DataColumnSidecarsForReconstruct(
		blobKzgCommitments,
		signedBlockHeader,
		kzgCommitmentsInclusionProof,
		recoveredCellsAndProofs,
	)
	if err != nil {
		log.WithError(err).Error("Failed to reconstruct all data column sidecars")
		return nil, errors.Wrapf(ErrReconstructionFailed, "peerdas.DataColumnSidecarsForReconstruct failed: %v", err)
	}

	// Verify all reconstructed sidecars.
	roColumns := make([]blocks.RODataColumn, len(pbSidecars))
	for i := range pbSidecars {
		roColumns[i], err = blocks.NewRODataColumn(pbSidecars[i])
		if err != nil {
			return nil, errors.Wrapf(ErrReconstructionFailed, "failed to create RODataColumn from reconstructed sidecar index %d: %v", pbSidecars[i].Index, err)
		}
	}

	verifiedSidecars, err := verifyColumnsForBlock(block, roColumns, newColumnsVerifier)
	if err != nil {
		return nil, errors.Wrap(err, "verify columns for block")
	}

	// Convert all reconstructed sidecars back to VerifiedRODataColumn.
	verifiedColsByIndex := make(map[uint64]blocks.VerifiedRODataColumn, len(verifiedSidecars))
	for _, verifiedSidecar := range verifiedSidecars {
		verifiedColsByIndex[verifiedSidecar.Index] = verifiedSidecar
	}

	// Filter to return only the originally requested columns.
	resultColumns := make([]blocks.VerifiedRODataColumn, 0, len(dataColumnsToFetch))
	missingRecovered := make(map[uint64]bool)
	for _, colIdx := range dataColumnsToFetch {
		if recoveredCol, ok := verifiedColsByIndex[colIdx]; ok {
			resultColumns = append(resultColumns, recoveredCol)
		} else {
			// This indicates a potential issue with reconstruction or the returned data.
			missingRecovered[colIdx] = true
		}
	}

	if len(missingRecovered) > 0 {
		log.WithField("missing", uint64MapToSortedSlice(missingRecovered)).Error("Reconstruction succeeded, but requested columns are missing from the result")
		return nil, errors.Wrapf(ErrReconstructionFailed, "reconstruction succeeded, but requested columns %v are missing from the result", uint64MapToSortedSlice(missingRecovered))
	}

	log.Info("Successfully reconstructed and recovered requested data columns")
	return resultColumns, nil
}

// RequestMissingDataColumnsByRange is an opinionated, high level function which, for each block in `blks`:
//   - Computes all data column sidecars we should store and which are missing (according to our node ID and `groupCount`),
//   - Builds an optimized set of data column sidecars by range requests in order to never request a data column that is already stored in the DB,
//     and in order to minimize the number of total requests, while not exceeding `batchSize` sidecars per requests.
//   - Greedily selects, among `peers`, the peers that can provide the requested data columns, to minimize the number of requests.
//   - Request the data column sidecars from the selected peers.
//   - In case of peers unable to actually provide all the requested data columns, retry with other peers.
//
// This function:
//   - returns on success when all the initially missing sidecars for `blks` are retrieved, or
//   - returns an error if no progress at all is made after 5 consecutives trials.
//     (If at least one additional data column sidecar is retrieved between two trials, the counter is reset.)
//
// In case of success, initially missing data columns grouped by block root are returned.
// This function expects blocks to be sorted by slot.
//
// TODO: In case at least one column is still missing after all allowed retries,
//
//	but `peers` custody more than 64 columns, then try to fetch enough columns to reconstruct needed ones.
func RequestMissingDataColumnsByRange(
	ctx context.Context,
	clock *startup.Clock,
	ctxMap ContextByteVersions,
	p2p p2p.P2P,
	rateLimiter *leakybucket.Collector,
	groupCount uint64,
	dataColumnsStorage filesystem.DataColumnStorageSummarizer,
	peers []peer.ID,
	blks []blocks.ROBlock,
	batchSize int,
) (map[[fieldparams.RootLength]byte][]blocks.RODataColumn, error) {
	const maxAllowedStall = 5 // Number of trials before giving up.

	if len(blks) == 0 {
		return nil, nil
	}

	// Get the current slot.
	currentSlot := clock.CurrentSlot()

	// Compute the minimum slot for which we should serve data columns.
	minimumSlot, err := DataColumnsRPCMinValidSlot(currentSlot)
	if err != nil {
		return nil, errors.Wrap(err, "data columns RPC min valid slot")
	}

	// Get blocks by root and compute all missing columns by root.
	blockByRoot := make(map[[fieldparams.RootLength]byte]blocks.ROBlock, len(blks))
	missingColumnsByRoot := make(map[[fieldparams.RootLength]byte]map[uint64]bool, len(blks))
	for _, blk := range blks {
		// Extract the block root and the block slot
		blockRoot, blockSlot := blk.Root(), blk.Block().Slot()

		// Populate the block by root.
		blockByRoot[blockRoot] = blk

		// Skip blocks that are not in the retention period.
		if blockSlot < minimumSlot {
			continue
		}

		missingColumns, err := MissingDataColumns(blk, p2p.NodeID(), groupCount, dataColumnsStorage)
		if err != nil {
			return nil, errors.Wrap(err, "missing data columns")
		}

		for _, column := range missingColumns {
			if _, ok := missingColumnsByRoot[blockRoot]; !ok {
				missingColumnsByRoot[blockRoot] = make(map[uint64]bool)
			}
			missingColumnsByRoot[blockRoot][column] = true
		}
	}

	// Return early if there are no missing data columns.
	if len(missingColumnsByRoot) == 0 {
		return nil, nil
	}

	// Compute the number of missing data columns.
	previousMissingDataColumnsCount := itemsCount(missingColumnsByRoot)

	// Count the number of retries for the same amount of missing data columns.
	stallCount := 0

	// Add log fields.
	log := log.WithFields(logrus.Fields{
		"initialMissingColumnsCount": previousMissingDataColumnsCount,
		"blockCount":                 len(blks),
		"firstSlot":                  blks[0].Block().Slot(),
		"lastSlot":                   blks[len(blks)-1].Block().Slot(),
	})

	// Log the start of the process.
	start := time.Now()
	log.Debug("Requesting data column sidecars - start")

	alignedDataColumnsByRoot := make(map[[fieldparams.RootLength]byte][]blocks.RODataColumn, len(blks))
	for len(missingColumnsByRoot) > 0 {
		// Build requests.
		requests, err := buildDataColumnByRangeRequests(blks, missingColumnsByRoot, batchSize)
		if err != nil {
			return nil, errors.Wrap(err, "build data column by range requests")
		}

		// Requests data column sidecars from peers.
		retrievedDataColumnsByRoot := make(map[[fieldparams.RootLength]byte][]blocks.RODataColumn)
		for _, request := range requests {
			roDataColumns, err := fetchDataColumnsFromPeers(ctx, clock, p2p, rateLimiter, ctxMap, peers, request)
			if err != nil {
				return nil, errors.Wrap(err, "fetch data columns from peers")
			}

			for _, roDataColumn := range roDataColumns {
				root := roDataColumn.BlockRoot()
				if _, ok := blockByRoot[root]; !ok {
					// It may happen if the peer which sent the data columns is on a different fork.
					continue
				}

				retrievedDataColumnsByRoot[root] = append(retrievedDataColumnsByRoot[root], roDataColumn)
			}
		}

		for root, dataColumns := range retrievedDataColumnsByRoot {
			// Retrieve the block from the root.
			block, ok := blockByRoot[root]
			if !ok {
				return nil, errors.New("block not found - this should never happen")
			}

			// Check if the data columns align with blocks.
			if err := verify.DataColumnsAlignWithBlock(block, dataColumns); err != nil {
				log.WithField("root", root).WithError(err).Debug("Data columns do not align with block")
				continue
			}

			alignedDataColumnsByRoot[root] = append(alignedDataColumnsByRoot[root], dataColumns...)

			// Remove aligned data columns from the missing columns.
			for _, dataColumn := range dataColumns {
				delete(missingColumnsByRoot[root], dataColumn.Index)
				if len(missingColumnsByRoot[root]) == 0 {
					delete(missingColumnsByRoot, root)
				}
			}
		}

		missingDataColumnsCount := itemsCount(missingColumnsByRoot)
		if missingDataColumnsCount == previousMissingDataColumnsCount {
			stallCount++
		} else {
			stallCount = 0
		}

		previousMissingDataColumnsCount = missingDataColumnsCount

		if missingDataColumnsCount > 0 {
			log := log.WithFields(logrus.Fields{
				"remainingMissingColumnsCount": missingDataColumnsCount,
				"stallCount":                   stallCount,
				"maxAllowedStall":              maxAllowedStall,
			})

			if stallCount >= maxAllowedStall {
				// It is very likely `bwbs` contains orphaned blocks, for which no peer has the data columns.
				// We give up and let the state machine handle the situation.
				const message = "Requesting data column sidecars - no progress, giving up"
				log.Warning(message)
				return nil, errors.New(message)
			}

			log.WithFields(logrus.Fields{
				"remainingMissingColumnsCount": missingDataColumnsCount,
				"stallCount":                   stallCount,
			}).Debug("Requesting data column sidecars - continue")
		}
	}

	log.WithField("duration", time.Since(start)).Debug("Requesting data column sidecars - success")
	return alignedDataColumnsByRoot, nil
}

// MissingDataColumns looks at the data columns we should store for a given block regarding `custodyGroupCount`,
// and returns the indices of the missing ones.
func MissingDataColumns(block blocks.ROBlock, nodeID enode.ID, custodyGroupCount uint64, dataColumnStorage filesystem.DataColumnStorageSummarizer) ([]uint64, error) {
	// Blocks before Fulu have no data columns.
	if block.Version() < version.Fulu {
		return nil, nil
	}

	// Get the blob commitments from the block.
	commitments, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob KZG commitments")
	}

	// Nothing to build if there are no commitments.
	if len(commitments) == 0 {
		return nil, nil
	}

	// Compute the expected columns.
	peerInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}

	expectedColumns := peerInfo.CustodyColumns

	// Get the stored columns.
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	summary := dataColumnStorage.Summary(block.Root())

	storedColumns := make(map[uint64]bool, numberOfColumns)
	for i := range numberOfColumns {
		if summary.HasIndex(i) {
			storedColumns[i] = true
		}
	}

	// Compute the missing columns.
	missingColumns := make([]uint64, 0, len(expectedColumns))
	for column := range expectedColumns {
		if !storedColumns[column] {
			missingColumns = append(missingColumns, column)
		}
	}

	return missingColumns, nil
}

// SelectPeersToFetchDataColumnsFrom implements greedy algorithm in order to select peers to fetch data columns from.
// https://en.wikipedia.org/wiki/Set_cover_problem#Greedy_algorithm
func SelectPeersToFetchDataColumnsFrom(neededDataColumns []uint64, dataColumnsByPeer map[peer.ID]map[uint64]bool) (map[peer.ID][]uint64, error) {
	// Copy the provided needed data columns into a set that we will remove elements from.
	remainingDataColumns := make(map[uint64]bool, len(neededDataColumns))
	for _, dataColumn := range neededDataColumns {
		remainingDataColumns[dataColumn] = true
	}

	dataColumnsFromSelectedPeers := make(map[peer.ID][]uint64)
	exhaustedPeers := make(map[peer.ID]bool)
	maxRequestDataColumnSidecars := params.BeaconConfig().MaxRequestDataColumnSidecars

	for len(remainingDataColumns) > 0 {
		// Select the peer that custody the most needed data columns (greedy selection).
		var bestPeer peer.ID
		maxCovered := 0

		for peer, dataColumns := range dataColumnsByPeer {
			if _, ok := dataColumnsFromSelectedPeers[peer]; ok {
				// Skip peers that have already been selected.
				continue
			}
			if _, ok := exhaustedPeers[peer]; ok {
				// Skip peers that have already been exhausted.
				continue
			}

			// Count how many *remaining* data columns this peer covers
			coveredCount := 0
			for col := range dataColumns {
				if remainingDataColumns[col] {
					coveredCount++
				}
			}

			// Update best peer if this one covers more *remaining* columns
			if coveredCount > maxCovered {
				maxCovered = coveredCount
				bestPeer = peer
			}

			if coveredCount == 0 {
				exhaustedPeers[peer] = true
			}
		}

		// No available peer covers any remaining needed column.
		if maxCovered == 0 {
			missingDataColumnsSortedSlice := uint64MapToSortedSlice(remainingDataColumns)
			// Return an instance of the specific error type, not wrapped.
			return nil, NewUnavailableColumnsError(missingDataColumnsSortedSlice)
		}

		// Get the actual columns this best peer provides from the set we still need.
		columnsProvidedByBestPeer := make([]uint64, 0)
		for col := range dataColumnsByPeer[bestPeer] {
			if remainingDataColumns[col] {
				columnsProvidedByBestPeer = append(columnsProvidedByBestPeer, col)
			}
		}
		// Sort for deterministic selection if truncated
		sort.Slice(columnsProvidedByBestPeer, func(i, j int) bool { return columnsProvidedByBestPeer[i] < columnsProvidedByBestPeer[j] })

		// Limit the request size per peer.
		if uint64(len(columnsProvidedByBestPeer)) > maxRequestDataColumnSidecars {
			columnsProvidedByBestPeer = columnsProvidedByBestPeer[:maxRequestDataColumnSidecars]
		}
		dataColumnsFromSelectedPeers[bestPeer] = columnsProvidedByBestPeer

		// Remove the columns covered by the selected peer from the remaining set.
		for _, dataColumn := range columnsProvidedByBestPeer {
			delete(remainingDataColumns, dataColumn)
		}
	}

	return dataColumnsFromSelectedPeers, nil
}

// AdmissiblePeersForDataColumns returns a map of peers that custody at least one data column listed in `neededDataColumns`.
//
// It returns:
// - A map, where the key of the map is the peer, the value is the custody groups of the peer.
// - A map, where the key of the map is the custody group, the value is a list of peers that custody the group.
// - A slice of descriptions for non admissible peers.
// - An error if any.
//
// NOTE: distributeSamplesToPeer from the DataColumnSampler implements similar logic,
// but with only one column queried in each request.
func AdmissiblePeersForDataColumns(
	peers []peer.ID,
	neededDataColumns []uint64,
	p2p p2p.P2P,
) (map[peer.ID]map[uint64]bool, map[uint64][]peer.ID, []string, error) {
	peerCount := len(peers)
	neededDataColumnsCount := uint64(len(neededDataColumns))

	// Create description slice for non admissible peers.
	descriptions := make([]string, 0, peerCount)

	// Compute custody columns for each peer.
	dataColumnsByPeer, err := custodyColumnsFromPeers(peers, p2p)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "custody columns from peers")
	}

	// Filter peers which custody at least one needed data column.
	dataColumnsByAdmissiblePeer, localDescriptions := filterPeerWhichCustodyAtLeastOneDataColumn(neededDataColumns, dataColumnsByPeer)
	descriptions = append(descriptions, localDescriptions...)

	// Compute a map from needed data columns to their peers.
	admissiblePeersByDataColumn := make(map[uint64][]peer.ID, neededDataColumnsCount)
	for peerId, peerDataColumns := range dataColumnsByAdmissiblePeer {
		for _, dataColumn := range neededDataColumns {
			if peerDataColumns[dataColumn] {
				admissiblePeersByDataColumn[dataColumn] = append(admissiblePeersByDataColumn[dataColumn], peerId)
			}
		}
	}

	return dataColumnsByAdmissiblePeer, admissiblePeersByDataColumn, descriptions, nil
}

// custodyGroupsFromPeer computes all the custody groups indexed by peer.
func custodyGroupsFromPeers(peers []peer.ID, p2pIface p2p.P2P) (map[peer.ID]map[uint64]bool, error) {
	peerCount := len(peers)

	custodyGroupsByPeer := make(map[peer.ID]map[uint64]bool, peerCount)
	for _, peer := range peers {
		// Get the node ID from the peer ID.
		nodeID, err := p2p.ConvertPeerIDToNodeID(peer)
		if err != nil {
			return nil, errors.Wrap(err, "convert peer ID to node ID")
		}

		// Get the custody group count of the peer.
		custodyGroupCount := p2pIface.CustodyGroupCountFromPeer(peer)

		// Get the custody groups of the peer.
		dasInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
		if err != nil {
			return nil, errors.Wrap(err, "custody groups")
		}

		custodyGroupsByPeer[peer] = dasInfo.CustodyGroups
	}

	return custodyGroupsByPeer, nil
}

// custodyColumnsFromPeers computes all the custody columns indexed by peer.
func custodyColumnsFromPeers(peers []peer.ID, p2p p2p.P2P) (map[peer.ID]map[uint64]bool, error) {
	// Get the custody groups of the peers.
	custodyGroupsByPeer, err := custodyGroupsFromPeers(peers, p2p)
	if err != nil {
		return nil, errors.Wrap(err, "custody groups from peer")
	}

	// Compute the custody columns of the peers.
	dataColumnsByPeer := make(map[peer.ID]map[uint64]bool, len(custodyGroupsByPeer))
	for peer, custodyGroups := range custodyGroupsByPeer {
		custodyColumns, err := peerdas.CustodyColumns(custodyGroups)
		if err != nil {
			return nil, errors.Wrap(err, "custody columns")
		}

		dataColumnsByPeer[peer] = custodyColumns
	}

	return dataColumnsByPeer, nil
}

// `filterPeerWhichCustodyAtLeastOneDataColumn` filters peers which custody at least one data column
// specified in `neededDataColumns`. It returns also a list of descriptions for non admissible peers.
func filterPeerWhichCustodyAtLeastOneDataColumn(neededDataColumns []uint64, inputDataColumnsByPeer map[peer.ID]map[uint64]bool) (map[peer.ID]map[uint64]bool, []string) {
	// Get the count of needed data columns.
	neededDataColumnsCount := uint64(len(neededDataColumns))

	// Create pretty needed data columns for logs.
	var neededDataColumnsLog interface{} = "all"
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	if neededDataColumnsCount < numberOfColumns {
		neededDataColumnsLog = neededDataColumns
	}

	outputDataColumnsByPeer := make(map[peer.ID]map[uint64]bool, len(inputDataColumnsByPeer))
	descriptions := make([]string, 0)

outerLoop:
	for peer, peerCustodyDataColumns := range inputDataColumnsByPeer {
		for _, neededDataColumn := range neededDataColumns {
			if peerCustodyDataColumns[neededDataColumn] {
				outputDataColumnsByPeer[peer] = peerCustodyDataColumns

				continue outerLoop
			}
		}

		peerCustodyColumnsCount := uint64(len(peerCustodyDataColumns))
		var peerCustodyColumnsLog interface{} = "all"

		if peerCustodyColumnsCount < numberOfColumns {
			peerCustodyColumnsLog = uint64MapToSortedSlice(peerCustodyDataColumns)
		}

		description := fmt.Sprintf(
			"peer %s: does not custody any needed column, custody columns: %v, needed columns: %v",
			peer, peerCustodyColumnsLog, neededDataColumnsLog,
		)

		descriptions = append(descriptions, description)
	}

	return outputDataColumnsByPeer, descriptions
}

// getBestPeers returns the list of best peers based on finalized checkpoint epoch.
func getBestPeers(p2p p2p.P2P, chain blockchain.FinalizationFetcher) []core.PeerID {
	_, bestPeers := p2p.Peers().BestFinalized(maxPeerRequest, chain.FinalizedCheckpt().Epoch)
	return bestPeers
}

// buildDataColumnByRangeRequests builds an optimized slices of data column by range requests:
// 1. It will never request a data column that is already stored in the DB if there is no "hole" in `roBlocks` other than missed slots.
// 2. It will minimize the number of requests.
// It expects blocks to be sorted by slot.
func buildDataColumnByRangeRequests(roBlocks []blocks.ROBlock, missingColumnsByRoot map[[fieldparams.RootLength]byte]map[uint64]bool, batchSize int) ([]*eth.DataColumnSidecarsByRangeRequest, error) {
	batchSizeSlot := primitives.Slot(batchSize)

	// Return early if there are no blocks to process.
	if len(roBlocks) == 0 {
		return nil, nil
	}

	// It's safe to get the first item of the slice since we've already checked that it's not empty.
	firstROBlock, lastROBlock := roBlocks[0], roBlocks[len(roBlocks)-1]
	firstBlockSlot, lastBlockSlot := firstROBlock.Block().Slot(), lastROBlock.Block().Slot()
	firstBlockRoot := firstROBlock.Root()

	previousMissingDataColumns := make(map[uint64]bool, len(missingColumnsByRoot[firstBlockRoot]))

	if missing, ok := missingColumnsByRoot[firstBlockRoot]; ok {
		for key, value := range missing {
			previousMissingDataColumns[key] = value
		}
	}

	previousBlockSlot, previousStartBlockSlot := firstBlockSlot, firstBlockSlot

	result := make([]*eth.DataColumnSidecarsByRangeRequest, 0, 1)
	for index := 1; index < len(roBlocks); index++ {
		roBlock := roBlocks[index]

		// Extract the block from the RO-block.
		block := roBlock.Block()

		// Extract the slot from the block.
		blockRoot, blockSlot := roBlock.Root(), block.Slot()

		if blockSlot <= previousBlockSlot {
			return nil, errors.Errorf("blocks are not strictly sorted by slot. Previous block slot: %d, current block slot: %d", previousBlockSlot, blockSlot)
		}

		// Extract KZG commitments count from the current block body
		blockKzgCommitments, err := block.Body().BlobKzgCommitments()
		if err != nil {
			return nil, errors.Wrap(err, "blob KZG commitments")
		}

		// Compute the count of KZG commitments.
		blockKzgCommitmentCount := len(blockKzgCommitments)

		// Skip blocks without commitments.
		if blockKzgCommitmentCount == 0 {
			previousBlockSlot = blockSlot
			continue
		}

		// Get the missing data columns for the current block.
		missingDataColumns := make(map[uint64]bool, len(missingColumnsByRoot[blockRoot]))
		for key, value := range missingColumnsByRoot[blockRoot] {
			missingDataColumns[key] = value
		}

		// Compute if the missing data columns differ.
		missingDataColumnsDiffer := uint64MapDiffer(previousMissingDataColumns, missingDataColumns)

		// Compute if the batch size is reached.
		batchSizeReached := blockSlot-previousStartBlockSlot >= batchSizeSlot

		if missingDataColumnsDiffer || batchSizeReached {
			// Append the slice to the result.
			request := &eth.DataColumnSidecarsByRangeRequest{
				StartSlot: previousStartBlockSlot,
				Count:     uint64(blockSlot - previousStartBlockSlot),
				Columns:   sortedSliceFromMap(previousMissingDataColumns),
			}

			result = append(result, request)

			previousStartBlockSlot, previousMissingDataColumns = blockSlot, missingDataColumns
		}

		previousBlockSlot = blockSlot
	}

	lastRequest := &eth.DataColumnSidecarsByRangeRequest{
		StartSlot: previousStartBlockSlot,
		Count:     uint64(lastBlockSlot - previousStartBlockSlot + 1),
		Columns:   sortedSliceFromMap(previousMissingDataColumns),
	}

	result = append(result, lastRequest)

	return result, nil
}

// fetchDataColumnsFromPeers requests data columns by range to relevant peers
func fetchDataColumnsFromPeers(
	ctx context.Context,
	clock *startup.Clock,
	p2p p2p.P2P,
	rateLimiter *leakybucket.Collector,
	ctxMap ContextByteVersions,
	peers []peer.ID,
	targetRequest *eth.DataColumnSidecarsByRangeRequest,
) ([]blocks.RODataColumn, error) {
	// Filter out requests with no data columns.
	if len(targetRequest.Columns) == 0 {
		return nil, nil
	}

	// Get all admissible peers with the data columns they custody.
	dataColumnsByAdmissiblePeer, err := waitForPeersForDataColumns(p2p, rateLimiter, peers, targetRequest)
	if err != nil {
		return nil, errors.Wrap(err, "wait for peers for data columns")
	}

	// Select the peers that will be requested.
	dataColumnsToFetchByPeer, err := SelectPeersToFetchDataColumnsFrom(targetRequest.Columns, dataColumnsByAdmissiblePeer)
	if err != nil {
		// This should never happen.
		return nil, errors.Wrap(err, "select peers to fetch data columns from")
	}

	var roDataColumns []blocks.RODataColumn
	for peer, columnsToFetch := range dataColumnsToFetchByPeer {
		// Build the request.
		request := &eth.DataColumnSidecarsByRangeRequest{
			StartSlot: targetRequest.StartSlot,
			Count:     targetRequest.Count,
			Columns:   columnsToFetch,
		}

		peerRoDataColumns, err := SendDataColumnSidecarsByRangeRequest(ctx, clock, p2p, peer, ctxMap, request)
		if err != nil {
			return nil, errors.Wrap(err, "send data column sidecars by range request")
		}

		roDataColumns = append(roDataColumns, peerRoDataColumns...)
	}

	return roDataColumns, nil
}

// waitForPeersForDataColumns returns a map, where the key of the map is the peer, the value is the custody columns of the peer.
// It uses only peers
// - synced up to `lastSlot`, and
// - have bandwidth to serve `blockCount` blocks.
// It waits until at least one peer per data column is available.
func waitForPeersForDataColumns(p2p p2p.P2P, rateLimiter *leakybucket.Collector, peers []peer.ID, request *eth.DataColumnSidecarsByRangeRequest) (map[peer.ID]map[uint64]bool, error) {
	const delay = 5 * time.Second

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	// Build nice log fields.
	lastSlot := request.StartSlot.Add(request.Count).Sub(1)

	var neededDataColumnsLog interface{} = "all"
	neededDataColumnCount := uint64(len(request.Columns))
	if neededDataColumnCount < numberOfColumns {
		neededDataColumnsLog = request.Columns
	}

	log := log.WithFields(logrus.Fields{
		"start":             request.StartSlot,
		"targetSlot":        lastSlot,
		"neededDataColumns": neededDataColumnsLog,
	})

	// Keep only peers with head epoch greater than or equal to the epoch corresponding to the target slot, and
	// keep only peers with enough bandwidth.
	filteredPeers, descriptions, err := filterPeersByTargetSlotAndBandwidth(p2p, rateLimiter, peers, lastSlot, request.Count)
	if err != nil {
		return nil, errors.Wrap(err, "filter eers by target slot and bandwidth")
	}

	// Get the peers that are admissible for the data columns.
	dataColumnsByAdmissiblePeer, admissiblePeersByDataColumn, moreDescriptions, err := AdmissiblePeersForDataColumns(filteredPeers, request.Columns, p2p)
	if err != nil {
		return nil, errors.Wrap(err, "admissible peers for data columns")
	}

	descriptions = append(descriptions, moreDescriptions...)

	// Compute data columns without any peer.
	dataColumnsWithoutPeers := computeDataColumnsWithoutPeers(request.Columns, admissiblePeersByDataColumn)

	// Wait if no suitable peers are available.
	for len(dataColumnsWithoutPeers) > 0 {
		// Build a nice log fields.
		var dataColumnsWithoutPeersLog interface{} = "all"
		dataColumnsWithoutPeersCount := uint64(len(dataColumnsWithoutPeers))
		if dataColumnsWithoutPeersCount < numberOfColumns {
			dataColumnsWithoutPeersLog = uint64MapToSortedSlice(dataColumnsWithoutPeers)
		}

		log.WithField("columnsWithoutPeer", dataColumnsWithoutPeersLog).Warning("Fetch data columns from peers - no available peers, retrying later")
		for _, description := range descriptions {
			log.Debug(description)
		}

		for pid, peerDataColumns := range dataColumnsByAdmissiblePeer {
			var peerDataColumnsLog interface{} = "all"
			peerDataColumnsCount := uint64(len(peerDataColumns))
			if peerDataColumnsCount < numberOfColumns {
				peerDataColumnsLog = uint64MapToSortedSlice(peerDataColumns)
			}

			log.WithFields(logrus.Fields{
				"peer":            pid,
				"peerDataColumns": peerDataColumnsLog,
			}).Debug("Peer data columns")
		}

		time.Sleep(delay)

		// Filter for peers with head epoch greater than or equal to our target epoch for ByRange requests.
		filteredPeers, descriptions, err = filterPeersByTargetSlotAndBandwidth(p2p, rateLimiter, peers, lastSlot, request.Count)
		if err != nil {
			return nil, errors.Wrap(err, "filter peers by target slot and bandwidth")
		}

		// Get the peers that are admissible for the data columns.
		dataColumnsByAdmissiblePeer, admissiblePeersByDataColumn, moreDescriptions, err = AdmissiblePeersForDataColumns(filteredPeers, request.Columns, p2p)
		if err != nil {
			return nil, errors.Wrap(err, "admissible peers for data columns")
		}

		descriptions = append(descriptions, moreDescriptions...)

		// Compute data columns without any peer.
		dataColumnsWithoutPeers = computeDataColumnsWithoutPeers(request.Columns, admissiblePeersByDataColumn)
	}

	return dataColumnsByAdmissiblePeer, nil
}

// Filter peers to ensure they are synced to the target slot and have sufficient bandwidth to serve the request.
func filterPeersByTargetSlotAndBandwidth(p2p p2p.P2P, rateLimiter *leakybucket.Collector, peers []peer.ID, lastSlot primitives.Slot, blockCount uint64) ([]peer.ID, []string, error) {
	if len(peers) == 0 {
		peers = p2p.Peers().Connected()
	}

	slotPeers, descriptions, err := filterPeersByTargetSlot(p2p, peers, lastSlot)
	if err != nil {
		return nil, nil, errors.Wrap(err, "peers with slot and data columns")
	}

	// Filter for peers with sufficient bandwidth to serve the request.
	slotAndBandwidthPeers := hasSufficientBandwidth(rateLimiter, slotPeers, blockCount)

	// Add debugging logs for the filtered peers.
	peerWithSufficientBandwidthMap := make(map[peer.ID]bool, len(peers))
	for _, peer := range slotAndBandwidthPeers {
		peerWithSufficientBandwidthMap[peer] = true
	}

	for _, peer := range slotPeers {
		if !peerWithSufficientBandwidthMap[peer] {
			description := fmt.Sprintf("peer %s: does not have sufficient bandwidth", peer)
			descriptions = append(descriptions, description)
		}
	}
	return slotAndBandwidthPeers, descriptions, nil
}

func hasSufficientBandwidth(rateLimiter *leakybucket.Collector, peers []peer.ID, count uint64) []peer.ID {
	var filteredPeers []peer.ID

	for _, p := range peers {
		if uint64(rateLimiter.Remaining(p.String())) < count {
			continue
		}
		copiedP := p
		filteredPeers = append(filteredPeers, copiedP)
	}
	return filteredPeers
}

func computeDataColumnsWithoutPeers(neededColumns []uint64, peersByColumn map[uint64][]peer.ID) map[uint64]bool {
	result := make(map[uint64]bool)
	for _, column := range neededColumns {
		if _, ok := peersByColumn[column]; !ok {
			result[column] = true
		}
	}

	return result
}

// Filter peers with head epoch lower than our target epoch for ByRange requests.
func filterPeersByTargetSlot(p2p p2p.P2P, peers []peer.ID, targetSlot primitives.Slot) ([]peer.ID, []string, error) {
	filteredPeers := make([]peer.ID, 0, len(peers))
	descriptions := make([]string, 0, len(peers))
	// Compute the target epoch from the target slot.
	targetEpoch := slots.ToEpoch(targetSlot)

	for _, peer := range peers {
		peerChainState, err := p2p.Peers().ChainState(peer)
		if err != nil {
			description := fmt.Sprintf("peer %s: error: %s", peer, err)
			descriptions = append(descriptions, description)
			continue
		}

		if peerChainState == nil {
			description := fmt.Sprintf("peer %s: chain state is nil", peer)
			descriptions = append(descriptions, description)
			continue
		}

		peerHeadEpoch := slots.ToEpoch(peerChainState.HeadSlot)

		if peerHeadEpoch < targetEpoch {
			description := fmt.Sprintf("peer %s: peer head epoch %d < our target epoch %d", peer, peerHeadEpoch, targetEpoch)
			descriptions = append(descriptions, description)
			continue
		}

		filteredPeers = append(filteredPeers, peer)
	}

	return filteredPeers, descriptions, nil
}

// itemsCount returns the total count of items
func itemsCount(missingColumnsByRoot map[[fieldparams.RootLength]byte]map[uint64]bool) int {
	count := 0
	for _, columns := range missingColumnsByRoot {
		count += len(columns)
	}
	return count
}

// uint64MapDiffer returns true if the two maps differ.
func uint64MapDiffer(left, right map[uint64]bool) bool {
	if len(left) != len(right) {
		return true
	}

	for k := range left {
		if !right[k] {
			return true
		}
	}

	return false
}
