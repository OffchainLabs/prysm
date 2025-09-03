package das

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enode"
	errors "github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// LazilyPersistentStoreColumn is an implementation of AvailabilityStore to be used when batch syncing data columns.
// This implementation will hold any data columns passed to Persist until the IsDataAvailable is called for their
// block, at which time they will undergo full verification and be saved to the disk.
type LazilyPersistentStoreColumn struct {
	store                  *filesystem.DataColumnStorage
	cache                  *dataColumnCache
	newDataColumnsVerifier verification.NewDataColumnsVerifier
	custody                *custodyRequirement
	bisector               Bisector
}

var _ AvailabilityChecker = &LazilyPersistentStoreColumn{}

// DataColumnsVerifier enables LazilyPersistentStoreColumn to manage the verification process
// going from RODataColumn->VerifiedRODataColumn, while avoiding the decision of which individual verifications
// to run and in what order. Since LazilyPersistentStoreColumn always tries to verify and save data columns only when
// they are all available, the interface takes a slice of data column sidecars.
type DataColumnsVerifier interface {
	VerifiedRODataColumns(ctx context.Context, blk blocks.ROBlock, scs []blocks.RODataColumn) ([]blocks.VerifiedRODataColumn, error)
}

// NewLazilyPersistentStoreColumn creates a new LazilyPersistentStoreColumn.
// WARNING: The resulting LazilyPersistentStoreColumn is NOT thread-safe.
func NewLazilyPersistentStoreColumn(
	store *filesystem.DataColumnStorage,
	newDataColumnsVerifier verification.NewDataColumnsVerifier,
	nodeID enode.ID,
	cgc uint64,
	bisector Bisector,
) *LazilyPersistentStoreColumn {
	return &LazilyPersistentStoreColumn{
		store:                  store,
		cache:                  newDataColumnCache(),
		newDataColumnsVerifier: newDataColumnsVerifier,
		custody:                &custodyRequirement{nodeID: nodeID, cgc: cgc},
		bisector:               bisector,
	}
}

// PersistColumns adds columns to the working column cache. Columns stored in this cache will be persisted
// for at least as long as the node is running. Once IsDataAvailable succeeds, all columns referenced
// by the given block are guaranteed to be persisted for the remainder of the retention period.
func (s *LazilyPersistentStoreColumn) Persist(current primitives.Slot, sidecars ...blocks.RODataColumn) error {
	currentEpoch := slots.ToEpoch(current)
	for _, sidecar := range sidecars {
		if !params.WithinDAPeriod(slots.ToEpoch(sidecar.Slot()), currentEpoch) {
			continue
		}
		if err := s.cache.stash(sidecar); err != nil {
			return errors.Wrap(err, "stash DataColumnSidecar")
		}
	}

	return nil
}

// IsDataAvailable returns nil if all the commitments in the given block are persisted to the db and have been verified.
// DataColumnsSidecars already in the db are assumed to have been previously verified against the block.
func (s *LazilyPersistentStoreColumn) IsDataAvailable(ctx context.Context, current primitives.Slot, blks ...blocks.ROBlock) error {
	currentEpoch := slots.ToEpoch(current)

	toVerify := make([]blocks.RODataColumn, 0)
	for _, block := range blks {
		indices, err := s.required(block, currentEpoch)
		if err != nil {
			return errors.Wrapf(err, "full commitments to check with block root `%#x` and current slot `%d`", block.Root(), current)
		}
		if indices.Count() == 0 {
			continue
		}

		key := cacheKey{slot: block.Block().Slot(), root: block.Root()}
		entry := s.cache.ensure(key)
		toVerify, err = entry.append(toVerify, block.Root(), indicesNotStored(s.store.Summary(block.Root()), indices))
		if err != nil {
			return errors.Wrap(err, "entry filter")
		}
	}

	if err := s.verifyAndSave(toVerify); err != nil {
		log.Warn("Batch verification failed, bisecting columns by peer")
		return s.bisectVerification(toVerify)
	}

	s.cache.cleanup(blks...)
	return nil
}

// required returns the set of column indices to check for a given block.
func (s *LazilyPersistentStoreColumn) required(block blocks.ROBlock, current primitives.Epoch) (peerdas.ColumnIndices, error) {
	eBlk := slots.ToEpoch(block.Block().Slot())
	eFulu := params.BeaconConfig().FuluForkEpoch
	if current < eFulu || eBlk < eFulu || !params.WithinDAPeriod(eBlk, current) {
		return peerdas.NewColumnIndices(), nil
	}

	commitments, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob KZG commitments")
	}
	// No DA check needed if the block has no blobs.
	if len(commitments) == 0 {
		return peerdas.NewColumnIndices(), nil
	}

	return s.custody.required(current)
}

func (s *LazilyPersistentStoreColumn) verifyAndSave(columns []blocks.RODataColumn) error {
	verified, err := s.verifyColumns(columns)
	if err != nil {
		return errors.Wrap(err, "verify columns")
	}
	if err := s.store.Save(verified); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	return nil
}

func (s *LazilyPersistentStoreColumn) verifyColumns(columns []blocks.RODataColumn) ([]blocks.VerifiedRODataColumn, error) {
	verifier := s.newDataColumnsVerifier(columns, verification.ByRangeRequestDataColumnSidecarRequirements)
	if err := verifier.ValidFields(); err != nil {
		return nil, errors.Wrap(err, "valid fields")
	}
	if err := verifier.SidecarInclusionProven(); err != nil {
		return nil, errors.Wrap(err, "sidecar inclusion proven")
	}
	if err := verifier.SidecarKzgProofVerified(); err != nil {
		return nil, errors.Wrap(err, "sidecar KZG proof verified")
	}

	return verifier.VerifiedRODataColumns()
}

func (s *LazilyPersistentStoreColumn) bisectVerification(columns []blocks.RODataColumn) error {
	if len(columns) == 0 {
		return nil
	}
	if s.bisector == nil {
		return errors.New("bisector not initialized")
	}
	if err := s.bisector.Bisect(columns); err != nil {
		return errors.Wrap(err, "Bisector.Bisect")
	}
	for columns, err := s.bisector.Next(); columns != nil; columns, err = s.bisector.Next() {
		if err != nil {
			break
		}
		if err := s.verifyAndSave(s.columnsNotStored(columns)); err != nil {
			s.bisector.OnError(err)
			continue
		}
	}
	return s.bisector.Error()
}

// columnsNotStored filters the list of ROColumnSidecars to only include those that are not found in the storage summary.
func (s *LazilyPersistentStoreColumn) columnsNotStored(sidecars []blocks.RODataColumn) []blocks.RODataColumn {
	// We use this method to filter a set of sidecars that were previously seen to be unavailable on disk. So our base assumption
	// is that they are still available and we don't need to copy the list. Instead we make a slice of any indices that are unexpectedly
	// stored and only when we find that the storage view has changed do we need to create a new slice.
	stored := make(map[int]struct{}, 0)
	lastRoot := [32]byte{}
	var sum filesystem.DataColumnStorageSummary
	for i, sc := range sidecars {
		if sc.BlockRoot() != lastRoot {
			sum = s.store.Summary(sc.BlockRoot())
		}
		if sum.HasIndex(sc.Index) {
			stored[i] = struct{}{}
		}
	}
	// If the view on storage hasn't changed, return the original list.
	if len(stored) == 0 {
		return sidecars
	}
	shift := 0
	for i := range sidecars {
		if _, ok := stored[i]; ok {
			// If the index is stored, skip and overwrite it.
			// Track how many spaces down to shift unseen sidecars (to overwrite the previously shifted or seen).
			shift++
			continue
		}
		if shift > 0 {
			// If the index is not stored and we have seen stored indices,
			// we need to shift the current index down.
			sidecars[i-shift] = sidecars[i]
		}
	}
	return sidecars[:len(sidecars)-shift]
}

type custodyRequirement struct {
	nodeID      enode.ID
	cgc         uint64 // custody group count
	current     primitives.Epoch
	requirement peerdas.ColumnIndices
}

func (c *custodyRequirement) required(current primitives.Epoch) (peerdas.ColumnIndices, error) {
	if c.current != current {
		peerInfo, _, err := peerdas.Info(c.nodeID, max(c.cgc, params.BeaconConfig().SamplesPerSlot))
		if err != nil {
			return peerdas.NewColumnIndices(), errors.Wrap(err, "peer info")
		}
		c.requirement = peerdas.NewColumnIndicesFromMap(peerInfo.CustodyColumns)
		c.current = current
	}
	return c.requirement, nil
}
