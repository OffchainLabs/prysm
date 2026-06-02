package filesystem

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/async/event"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/io/file"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/spf13/afero"
)

const (
	proofVersion         = 0x01
	proofVersionSize     = 1                                       // bytes
	maxProofTypes        = 8                                       // ExecutionProofId max value (EXECUTION_PROOF_TYPE_COUNT)
	proofOffsetSize      = 4                                       // bytes for offset (uint32)
	proofSizeSize        = 4                                       // bytes for size (uint32)
	proofSlotSize        = proofOffsetSize + proofSizeSize         // 8 bytes per slot
	proofOffsetTableSize = maxProofTypes * proofSlotSize           // 64 bytes
	proofHeaderSize      = proofVersionSize + proofOffsetTableSize // 65 bytes
	proofsFileExtension  = "sszs"
	proofPrunePeriod     = 1 * time.Minute
)

var (
	errProofIDTooLarge        = errors.New("proof ID too large")
	errWrongProofBytesWritten = errors.New("wrong number of bytes written")
	errWrongProofVersion      = errors.New("wrong proof version")
	errWrongProofBytesRead    = errors.New("wrong number of bytes read")
	errNoProofBasePath        = errors.New("ProofStorage base path not specified in init")
	errProofAlreadyExists     = errors.New("proof already exists")
)

type (
	// ProofIdent is a unique identifier for a proof.
	ProofIdent struct {
		BlockRoot [fieldparams.RootLength]byte
		Epoch     primitives.Epoch
		ProofType uint8
	}

	// ProofsIdent is a collection of unique identifiers for proofs.
	ProofsIdent struct {
		BlockRoot  [fieldparams.RootLength]byte
		Epoch      primitives.Epoch
		ProofTypes []uint8
	}

	// ProofStorage is the concrete implementation of the filesystem backend for saving and retrieving ExecutionProofs.
	ProofStorage struct {
		base                   string
		finalizedEpochProvider FinalizedEpochProvider
		fs                     afero.Fs
		cache                  *proofCache
		proofFeed              *event.Feed
		pruneMu                sync.RWMutex

		mu      sync.Mutex // protects muChans
		muChans map[[fieldparams.RootLength]byte]*proofMuChan
	}

	// ProofStorageOption is a functional option for configuring a ProofStorage.
	ProofStorageOption func(*ProofStorage) error

	// FinalizedEpochProvider returns the current finalized epoch as seen by the
	// chain. When configured on a ProofStorage, pruning will never remove
	// proofs whose block epoch is greater than or equal to the finalized epoch,
	// guaranteeing that unfinalized blocks always have their proofs available.
	FinalizedEpochProvider func() primitives.Epoch

	proofMuChan struct {
		mu      *sync.RWMutex
		toStore chan []blocks.VerifiedROSignedExecutionProof
	}

	// proofSlotEntry represents the offset and size for a proof in the file.
	proofSlotEntry struct {
		offset uint32
		size   uint32
	}

	// proofOffsetTable is the offset table with 8 slots indexed by proofID.
	proofOffsetTable [maxProofTypes]proofSlotEntry

	// proofFileMetadata contains metadata extracted from a proof file path.
	proofFileMetadata struct {
		period    uint64
		epoch     primitives.Epoch
		blockRoot [fieldparams.RootLength]byte
	}
)

// WithProofBasePath is a required option that sets the base path of proof storage.
func WithProofBasePath(base string) ProofStorageOption {
	return func(ps *ProofStorage) error {
		ps.base = base
		return nil
	}
}

// WithProofFs allows the afero.Fs implementation to be customized.
// Used by tests to substitute an in-memory filesystem.
func WithProofFs(fs afero.Fs) ProofStorageOption {
	return func(ps *ProofStorage) error {
		ps.fs = fs
		return nil
	}
}

// WithProofFinalizedEpochProvider configures a hook returning the current
// finalized epoch. The pruner uses it to delete proofs for finalized blocks
// only: anything at or above the finalized epoch is retained, guaranteeing
// that unfinalized blocks always have their proofs available.
func WithProofFinalizedEpochProvider(p FinalizedEpochProvider) ProofStorageOption {
	return func(ps *ProofStorage) error {
		ps.finalizedEpochProvider = p
		return nil
	}
}

// NewProofStorage creates a new instance of the ProofStorage object.
func NewProofStorage(ctx context.Context, opts ...ProofStorageOption) (*ProofStorage, error) {
	storage := &ProofStorage{
		proofFeed: new(event.Feed),
		muChans:   make(map[[fieldparams.RootLength]byte]*proofMuChan),
	}

	for _, o := range opts {
		if err := o(storage); err != nil {
			return nil, fmt.Errorf("failed to create proof storage: %w", err)
		}
	}

	// Allow tests to set up a different fs using WithProofFs.
	if storage.fs == nil {
		if storage.base == "" {
			return nil, errNoProofBasePath
		}

		storage.base = path.Clean(storage.base)
		if err := file.MkdirAll(storage.base); err != nil {
			return nil, fmt.Errorf("failed to create proof storage at %s: %w", storage.base, err)
		}

		storage.fs = afero.NewBasePathFs(afero.NewOsFs(), storage.base)
	}

	storage.cache = newProofCache()

	async.RunEvery(ctx, proofPrunePeriod, func() {
		storage.pruneMu.Lock()
		defer storage.pruneMu.Unlock()

		storage.prune()
	})

	return storage, nil
}

// WarmCache warms the cache of the proof filesystem.
func (ps *ProofStorage) WarmCache() {
	start := time.Now()
	log.Info("Proof filesystem cache warm-up started")

	ps.pruneMu.Lock()
	defer ps.pruneMu.Unlock()

	// List all period directories
	periodFileInfos, err := afero.ReadDir(ps.fs, ".")
	if err != nil {
		log.WithError(err).Error("Error reading top directory during proof warm cache")
		return
	}

	// Iterate through periods
	for _, periodFileInfo := range periodFileInfos {
		if !periodFileInfo.IsDir() {
			continue
		}

		periodPath := periodFileInfo.Name()

		// List all epoch directories in this period
		epochFileInfos, err := afero.ReadDir(ps.fs, periodPath)
		if err != nil {
			log.WithError(err).WithField("period", periodPath).Error("Error reading period directory during proof warm cache")
			continue
		}

		// Iterate through epochs
		for _, epochFileInfo := range epochFileInfos {
			if !epochFileInfo.IsDir() {
				continue
			}

			epochPath := path.Join(periodPath, epochFileInfo.Name())

			// List all .sszs files in this epoch
			files, err := ps.listProofEpochFiles(epochPath)
			if err != nil {
				log.WithError(err).WithField("epoch", epochPath).Error("Error listing epoch files during proof warm cache")
				continue
			}

			// Process all files in this epoch in parallel
			ps.processProofEpochFiles(files)
		}
	}

	// Prune the cache and the filesystem
	ps.prune()

	totalElapsed := time.Since(start)
	log.WithField("elapsed", totalElapsed).Info("Proof filesystem cache warm-up complete")
}

// listProofEpochFiles lists all .sszs files in an epoch directory.
func (ps *ProofStorage) listProofEpochFiles(epochPath string) ([]string, error) {
	fileInfos, err := afero.ReadDir(ps.fs, epochPath)
	if err != nil {
		return nil, fmt.Errorf("read epoch directory: %w", err)
	}

	files := make([]string, 0, len(fileInfos))
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			continue
		}

		fileName := fileInfo.Name()
		if strings.HasSuffix(fileName, "."+proofsFileExtension) {
			files = append(files, path.Join(epochPath, fileName))
		}
	}

	return files, nil
}

// processProofEpochFiles processes all proof files in an epoch in parallel.
func (ps *ProofStorage) processProofEpochFiles(files []string) {
	var wg sync.WaitGroup

	for _, filePath := range files {
		wg.Go(func() {
			if err := ps.processProofFile(filePath); err != nil {
				log.WithError(err).WithField("file", filePath).Error("Error processing proof file during warm cache")
			}
		})
	}

	wg.Wait()
}

// processProofFile processes a single .sszs proof file for cache warming.
func (ps *ProofStorage) processProofFile(filePath string) error {
	// Extract metadata from the file path
	fileMetadata, err := extractProofFileMetadata(filePath)
	if err != nil {
		return fmt.Errorf("extract proof file metadata: %w", err)
	}

	// Open the file
	f, err := ps.fs.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}

	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.WithError(closeErr).WithField("file", filePath).Error("Error closing file during proof warm cache")
		}
	}()

	// Read the offset table
	offsetTable, _, err := ps.readHeader(f)
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}

	// Add all present proofs to the cache
	for proofID, entry := range offsetTable {
		if entry.size == 0 {
			continue
		}

		proofIdent := ProofIdent{
			BlockRoot: fileMetadata.blockRoot,
			Epoch:     fileMetadata.epoch,
			ProofType: uint8(proofID),
		}

		ps.cache.set(proofIdent)
	}

	return nil
}

// Summary returns the ProofStorageSummary for a given root.
func (ps *ProofStorage) Summary(root [fieldparams.RootLength]byte) ProofStorageSummary {
	return ps.cache.Summary(root)
}

// Save saves execution proofs into the database.
// The proofs must all belong to the same block (same block root).
func (ps *ProofStorage) Save(proofs []blocks.VerifiedROSignedExecutionProof) error {
	startTime := time.Now()

	if len(proofs) == 0 {
		return nil
	}

	// Safely retrieve the block root and the epoch.
	first := proofs[0]
	blockRoot := first.BlockRoot()
	epoch := first.Epoch()

	proofTypes := make([]uint8, 0, len(proofs))
	for _, proof := range proofs {
		// Check if the proof ID is valid.
		proofType := proof.Message.ProofType[0]
		if proofType >= maxProofTypes {
			return errProofIDTooLarge
		}

		// Save proofs in the filesystem.
		if err := ps.saveFilesystem(proof.BlockRoot(), proof.Epoch(), proofs); err != nil {
			return fmt.Errorf("save filesystem: %w", err)
		}

		proofTypes = append(proofTypes, proof.Message.ProofType[0])
	}

	// Compute the proofs ident.
	proofsIdent := ProofsIdent{BlockRoot: blockRoot, Epoch: epoch, ProofTypes: proofTypes}

	// Set proofs in the cache.
	ps.cache.setMultiple(proofsIdent)

	// Notify the proof feed.
	ps.proofFeed.Send(proofsIdent)

	proofSaveLatency.Observe(float64(time.Since(startTime).Milliseconds()))

	return nil
}

// saveFilesystem saves proofs into the database.
// This function expects all proofs to belong to the same block.
func (ps *ProofStorage) saveFilesystem(root [fieldparams.RootLength]byte, epoch primitives.Epoch, proofs []blocks.VerifiedROSignedExecutionProof) error {
	// Compute the file path.
	filePath := proofFilePath(root, epoch)

	ps.pruneMu.RLock()
	defer ps.pruneMu.RUnlock()

	fileMu, toStore := ps.fileMutexChan(root)
	toStore <- proofs

	fileMu.Lock()
	defer fileMu.Unlock()

	// Check if the file exists.
	exists, err := afero.Exists(ps.fs, filePath)
	if err != nil {
		return fmt.Errorf("afero exists: %w", err)
	}

	if exists {
		if err := ps.saveProofExistingFile(filePath, toStore); err != nil {
			return fmt.Errorf("save proof existing file: %w", err)
		}

		return nil
	}

	if err := ps.saveProofNewFile(filePath, toStore); err != nil {
		return fmt.Errorf("save proof new file: %w", err)
	}

	return nil
}

// Subscribe subscribes to the proof feed.
// It returns the subscription and a 1-size buffer channel to receive proof idents.
func (ps *ProofStorage) Subscribe() (event.Subscription, <-chan ProofsIdent) {
	identsChan := make(chan ProofsIdent, 1)
	subscription := ps.proofFeed.Subscribe(identsChan)
	return subscription, identsChan
}

// Get retrieves signed execution proofs from the database.
// If one of the requested proofs is not found, it is just skipped.
// If proofIDs is nil, then all stored proofs are returned.
func (ps *ProofStorage) Get(root [fieldparams.RootLength]byte, proofIDs []uint8) ([]*ethpb.SignedExecutionProof, error) {
	ps.pruneMu.RLock()
	defer ps.pruneMu.RUnlock()

	fileMu, _ := ps.fileMutexChan(root)
	fileMu.RLock()
	defer fileMu.RUnlock()

	startTime := time.Now()

	// Build all proofIDs if none are provided.
	if proofIDs == nil {
		proofIDs = make([]uint8, maxProofTypes)
		for i := range proofIDs {
			proofIDs[i] = uint8(i)
		}
	}

	summary, ok := ps.cache.get(root)
	if !ok {
		// Nothing found in db. Exit early.
		return nil, nil
	}

	// Check if any requested proofID exists.
	if !slices.ContainsFunc(proofIDs, summary.HasProof) {
		return nil, nil
	}

	// Compute the file path.
	filePath := proofFilePath(root, summary.epoch)

	// Open the proof file.
	file, err := ps.fs.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("proof file open: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.WithError(closeErr).WithField("file", filePath).Error("Error closing proof file")
		}
	}()

	// Read the header.
	offsetTable, _, err := ps.readHeader(file)
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Retrieve proofs from the file.
	proofs := make([]*ethpb.SignedExecutionProof, 0, len(proofIDs))
	for _, proofID := range proofIDs {
		if proofID >= maxProofTypes {
			continue
		}

		entry := offsetTable[proofID]
		// Skip if the proof is not saved.
		if entry.size == 0 {
			continue
		}

		// Seek to the proof offset (offset is relative to end of header).
		_, err = file.Seek(proofHeaderSize+int64(entry.offset), io.SeekStart)
		if err != nil {
			return nil, fmt.Errorf("seek: %w", err)
		}

		// Read the SSZ encoded proof.
		sszProof := make([]byte, entry.size)
		n, err := io.ReadFull(file, sszProof)
		if err != nil {
			return nil, fmt.Errorf("read proof: %w", err)
		}
		if n != int(entry.size) {
			return nil, errWrongProofBytesRead
		}

		// Unmarshal the signed proof.
		proof := new(ethpb.SignedExecutionProof)
		if err := proof.UnmarshalSSZ(sszProof); err != nil {
			return nil, fmt.Errorf("unmarshal proof: %w", err)
		}

		proofs = append(proofs, proof)
	}

	proofFetchLatency.Observe(float64(time.Since(startTime).Milliseconds()))

	return proofs, nil
}

// Remove deletes all proofs for a given root.
func (ps *ProofStorage) Remove(blockRoot [fieldparams.RootLength]byte) error {
	ps.pruneMu.RLock()
	defer ps.pruneMu.RUnlock()

	fileMu, _ := ps.fileMutexChan(blockRoot)
	fileMu.Lock()
	defer fileMu.Unlock()

	summary, ok := ps.cache.get(blockRoot)
	if !ok {
		// Nothing found in db. Exit early.
		return nil
	}

	// Remove the proofs from the cache.
	ps.cache.evict(blockRoot)

	// Remove the proof file.
	filePath := proofFilePath(blockRoot, summary.epoch)
	if err := ps.fs.Remove(filePath); err != nil {
		return fmt.Errorf("remove: %w", err)
	}

	return nil
}

// Clear deletes all files on the filesystem.
func (ps *ProofStorage) Clear() error {
	ps.pruneMu.Lock()
	defer ps.pruneMu.Unlock()

	dirs, err := listDir(ps.fs, ".")
	if err != nil {
		return fmt.Errorf("list dir: %w", err)
	}

	ps.cache.clear()

	for _, dir := range dirs {
		if err := ps.fs.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove all: %w", err)
		}
	}

	return nil
}

// saveProofNewFile saves proofs to a new file.
func (ps *ProofStorage) saveProofNewFile(filePath string, inputProofs chan []blocks.VerifiedROSignedExecutionProof) (err error) {
	// Initialize the offset table.
	var offsetTable proofOffsetTable

	var sszEncodedProofs []byte
	currentOffset := uint32(0)

	for {
		proofs := pullProofChan(inputProofs)
		if len(proofs) == 0 {
			break
		}

		for _, proof := range proofs {
			proofType := proof.Message.ProofType[0]
			if proofType >= maxProofTypes {
				continue
			}

			// Skip if already in offset table (duplicate).
			if offsetTable[proofType].size != 0 {
				continue
			}

			// SSZ encode the full signed proof.
			sszProof, err := proof.SignedExecutionProof.MarshalSSZ()
			if err != nil {
				return fmt.Errorf("marshal proof SSZ: %w", err)
			}

			proofSize := uint32(len(sszProof))

			// Update offset table.
			offsetTable[proofType] = proofSlotEntry{
				offset: currentOffset,
				size:   proofSize,
			}

			// Append SSZ encoded proof.
			sszEncodedProofs = append(sszEncodedProofs, sszProof...)
			currentOffset += proofSize
		}
	}

	if len(sszEncodedProofs) == 0 {
		// Nothing to save.
		return nil
	}

	// Create directory structure.
	dir := filepath.Dir(filePath)
	if err := ps.fs.MkdirAll(dir, directoryPermissions()); err != nil {
		return fmt.Errorf("mkdir all: %w", err)
	}

	file, err := ps.fs.Create(filePath)
	if err != nil {
		return fmt.Errorf("create proof file: %w", err)
	}

	defer func() {
		closeErr := file.Close()
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	// Build the file content.
	countToWrite := proofHeaderSize + len(sszEncodedProofs)
	bytes := make([]byte, 0, countToWrite)

	// Write version byte.
	bytes = append(bytes, byte(proofVersion))

	// Write offset table.
	bytes = append(bytes, encodeOffsetTable(offsetTable)...)

	// Write SSZ encoded proofs.
	bytes = append(bytes, sszEncodedProofs...)

	countWritten, err := file.Write(bytes)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if countWritten != countToWrite {
		return errWrongProofBytesWritten
	}

	syncStart := time.Now()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	proofFileSyncLatency.Observe(float64(time.Since(syncStart).Milliseconds()))

	return nil
}

// saveProofExistingFile saves proofs to an existing file.
func (ps *ProofStorage) saveProofExistingFile(filePath string, inputProofs chan []blocks.VerifiedROSignedExecutionProof) (err error) {
	// Open the file for read/write.
	file, err := ps.fs.OpenFile(filePath, os.O_RDWR, os.FileMode(0600))
	if err != nil {
		return fmt.Errorf("open proof file: %w", err)
	}

	defer func() {
		closeErr := file.Close()
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	// Read current header.
	offsetTable, fileSize, err := ps.readHeader(file)
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}

	var sszEncodedProofs []byte
	currentOffset := uint32(fileSize - proofHeaderSize)
	modified := false

	for {
		proofs := pullProofChan(inputProofs)
		if len(proofs) == 0 {
			break
		}

		for _, proof := range proofs {
			proofType := proof.Message.ProofType[0]
			if proofType >= maxProofTypes {
				continue
			}

			// Skip if proof already exists.
			if offsetTable[proofType].size != 0 {
				continue
			}

			// SSZ encode the full signed proof.
			sszProof, err := proof.SignedExecutionProof.MarshalSSZ()
			if err != nil {
				return fmt.Errorf("marshal proof SSZ: %w", err)
			}

			proofSize := uint32(len(sszProof))

			// Update offset table.
			offsetTable[proofType] = proofSlotEntry{
				offset: currentOffset,
				size:   proofSize,
			}

			// Append SSZ encoded proof.
			sszEncodedProofs = append(sszEncodedProofs, sszProof...)
			currentOffset += proofSize
			modified = true
		}
	}

	if !modified {
		return nil
	}

	// Write updated offset table back to file (at position 1, after version byte).
	encodedTable := encodeOffsetTable(offsetTable)
	count, err := file.WriteAt(encodedTable, int64(proofVersionSize))
	if err != nil {
		return fmt.Errorf("write offset table: %w", err)
	}
	if count != proofOffsetTableSize {
		return errWrongProofBytesWritten
	}

	// Append the SSZ encoded proofs to the end of the file.
	count, err = file.WriteAt(sszEncodedProofs, fileSize)
	if err != nil {
		return fmt.Errorf("write SSZ encoded proofs: %w", err)
	}
	if count != len(sszEncodedProofs) {
		return errWrongProofBytesWritten
	}

	syncStart := time.Now()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	proofFileSyncLatency.Observe(float64(time.Since(syncStart).Milliseconds()))

	return nil
}

// readHeader reads the file header and returns the offset table and file size.
func (ps *ProofStorage) readHeader(file afero.File) (proofOffsetTable, int64, error) {
	var header [proofHeaderSize]byte
	countRead, err := file.ReadAt(header[:], 0)
	if err != nil {
		return proofOffsetTable{}, 0, fmt.Errorf("read at: %w", err)
	}
	if countRead != proofHeaderSize {
		return proofOffsetTable{}, 0, errWrongProofBytesRead
	}

	// Check version.
	fileVersion := int(header[0])
	if fileVersion != proofVersion {
		return proofOffsetTable{}, 0, errWrongProofVersion
	}

	// Decode offset table and compute file size.
	var offsetTable proofOffsetTable
	fileSize := int64(proofHeaderSize)
	for i := range offsetTable {
		pos := proofVersionSize + i*proofSlotSize
		offsetTable[i].offset = binary.BigEndian.Uint32(header[pos : pos+proofOffsetSize])
		offsetTable[i].size = binary.BigEndian.Uint32(header[pos+proofOffsetSize : pos+proofSlotSize])
		fileSize += int64(offsetTable[i].size)
	}

	return offsetTable, fileSize, nil
}

// prune cleans the cache, the filesystem and mutexes.
// Only proofs for finalized blocks are removed: anything with epoch >= the
// current finalized epoch is retained, guaranteeing that unfinalized blocks
// always have their proofs available. If no finalized-epoch provider is wired
// (e.g. in unit tests), pruning is a no-op.
func (ps *ProofStorage) prune() {
	startTime := time.Now()
	defer func() {
		proofPruneLatency.Observe(float64(time.Since(startTime).Milliseconds()))
	}()

	if ps.finalizedEpochProvider == nil {
		return
	}
	finalizedEpoch := ps.finalizedEpochProvider()
	if finalizedEpoch == 0 {
		return
	}
	// Prune strictly below the finalized epoch: the finalized checkpoint
	// block and everything above it remain.
	highestEpochToPrune := finalizedEpoch - 1
	highestPeriodToPrune := proofPeriod(highestEpochToPrune)

	// Prune the cache.
	prunedCount := ps.cache.pruneUpTo(highestEpochToPrune)

	if prunedCount == 0 {
		return
	}

	proofPrunedCounter.Add(float64(prunedCount))

	// Prune the filesystem.
	periodFileInfos, err := afero.ReadDir(ps.fs, ".")
	if err != nil {
		log.WithError(err).Error("Error encountered while reading top directory during proof prune")
		return
	}

	for _, periodFileInfo := range periodFileInfos {
		periodStr := periodFileInfo.Name()
		period, err := strconv.ParseUint(periodStr, 10, 64)
		if err != nil {
			log.WithError(err).Errorf("Error encountered while parsing period %s", periodStr)
			continue
		}

		if period < highestPeriodToPrune {
			// Remove everything lower than highest period to prune.
			if err := ps.fs.RemoveAll(periodStr); err != nil {
				log.WithError(err).Error("Error encountered while removing period directory during proof prune")
			}
			continue
		}

		if period > highestPeriodToPrune {
			// Do not remove anything higher than highest period to prune.
			continue
		}

		// if period == highestPeriodToPrune
		epochFileInfos, err := afero.ReadDir(ps.fs, periodStr)
		if err != nil {
			log.WithError(err).Error("Error encountered while reading epoch directory during proof prune")
			continue
		}

		for _, epochFileInfo := range epochFileInfos {
			epochStr := epochFileInfo.Name()
			epochDir := path.Join(periodStr, epochStr)

			epoch, err := strconv.ParseUint(epochStr, 10, 64)
			if err != nil {
				log.WithError(err).Errorf("Error encountered while parsing epoch %s", epochStr)
				continue
			}

			if primitives.Epoch(epoch) > highestEpochToPrune {
				continue
			}

			if err := ps.fs.RemoveAll(epochDir); err != nil {
				log.WithError(err).Error("Error encountered while removing epoch directory during proof prune")
				continue
			}
		}
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()
	clear(ps.muChans)
}

// fileMutexChan returns the file mutex and channel for a given block root.
func (ps *ProofStorage) fileMutexChan(root [fieldparams.RootLength]byte) (*sync.RWMutex, chan []blocks.VerifiedROSignedExecutionProof) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	mc, ok := ps.muChans[root]
	if !ok {
		mc = &proofMuChan{
			mu:      new(sync.RWMutex),
			toStore: make(chan []blocks.VerifiedROSignedExecutionProof, 1),
		}
		ps.muChans[root] = mc
		return mc.mu, mc.toStore
	}

	return mc.mu, mc.toStore
}

// pullProofChan pulls proofs from the input channel until it is empty.
func pullProofChan(inputProofs chan []blocks.VerifiedROSignedExecutionProof) []blocks.VerifiedROSignedExecutionProof {
	proofs := make([]blocks.VerifiedROSignedExecutionProof, 0, maxProofTypes)

	for {
		select {
		case batch := <-inputProofs:
			proofs = append(proofs, batch...)
		default:
			return proofs
		}
	}
}

// proofFilePath builds the file path in database for a given root and epoch.
func proofFilePath(root [fieldparams.RootLength]byte, epoch primitives.Epoch) string {
	return path.Join(
		fmt.Sprintf("%d", proofPeriod(epoch)),
		fmt.Sprintf("%d", epoch),
		fmt.Sprintf("%#x.%s", root, proofsFileExtension),
	)
}

// extractProofFileMetadata extracts the metadata from a proof file path.
func extractProofFileMetadata(path string) (*proofFileMetadata, error) {
	// Use filepath.Separator to handle both Windows (\) and Unix (/) path separators
	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) != 3 {
		return nil, fmt.Errorf("unexpected proof file %s", path)
	}

	period, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse period from %s: %w", path, err)
	}

	epoch, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse epoch from %s: %w", path, err)
	}

	partsRoot := strings.Split(parts[2], ".")
	if len(partsRoot) != 2 {
		return nil, fmt.Errorf("failed to parse root from %s", path)
	}

	blockRootString := partsRoot[0]
	if len(blockRootString) != 2+2*fieldparams.RootLength || blockRootString[:2] != "0x" {
		return nil, fmt.Errorf("unexpected proof file name %s", path)
	}

	if partsRoot[1] != proofsFileExtension {
		return nil, fmt.Errorf("unexpected extension %s", path)
	}

	blockRootSlice, err := hex.DecodeString(blockRootString[2:])
	if err != nil {
		return nil, fmt.Errorf("decode string from %s: %w", path, err)
	}

	var blockRoot [fieldparams.RootLength]byte
	copy(blockRoot[:], blockRootSlice)

	result := &proofFileMetadata{period: period, epoch: primitives.Epoch(epoch), blockRoot: blockRoot}
	return result, nil
}

// proofPeriod computes the period of a given epoch.
func proofPeriod(epoch primitives.Epoch) uint64 {
	return uint64(epoch / params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)
}

// encodeOffsetTable encodes the offset table to bytes.
func encodeOffsetTable(table proofOffsetTable) []byte {
	result := make([]byte, proofOffsetTableSize)
	for i, entry := range table {
		offset := i * proofSlotSize
		binary.BigEndian.PutUint32(result[offset:offset+proofOffsetSize], entry.offset)
		binary.BigEndian.PutUint32(result[offset+proofOffsetSize:offset+proofSlotSize], entry.size)
	}
	return result
}
