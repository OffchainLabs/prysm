package filesystem

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/async"
	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
)

const (
	version                  = 0x01
	versionOffset            = 0                           // bytes
	versionSize              = 1                           // bytes
	sidecarByteLenOffset     = versionOffset + versionSize // (Offset of the encoded size of the SSZ encoded data column sidecar)
	sidecarByteLenSize       = 4                           // bytes (Size of the encoded size of the SSZ encoded data column sidecar)
	mandatoryNumberOfColumns = 128                         // 2**7
	indicesOffset            = sidecarByteLenOffset + sidecarByteLenSize
	nonZeroOffset            = mandatoryNumberOfColumns
	headerSize               = versionSize + sidecarByteLenSize + mandatoryNumberOfColumns
	dataColumnsFileExtension = "sszs"
	prunePeriod              = 1 * time.Minute
)

var (
	errWrongNumberOfColumns                 = errors.New("wrong number of data columns")
	errDataColumnIndexTooLarge              = errors.New("data column index too large")
	errWrongBytesWritten                    = errors.New("wrong number of bytes written")
	errWrongVersion                         = errors.New("wrong version")
	errWrongBytesHeaderRead                 = errors.New("wrong number of bytes header read")
	errTooManyDataColumns                   = errors.New("too many data columns")
	errWrongSszEncodedDataColumnSidecarSize = errors.New("wrong SSZ encoded data column sidecar size")
	errDataColumnSidecarsFromDifferentSlots = errors.New("data column sidecars from different slots")
	errNoDataColumnBasePath                 = errors.New("DataColumnStorage base path not specified in init")
)

type (
	// DataColumnStorage is the concrete implementation of the filesystem backend for saving and retrieving DataColumnSidecars.
	DataColumnStorage struct {
		base            string
		retentionEpochs primitives.Epoch
		fs              afero.Fs
		cache           *dataColumnStorageSummaryCache
		dataColumnFeed  *event.Feed
		pruneMu         sync.RWMutex

		mu      sync.Mutex // protects muChans
		muChans map[[fieldparams.RootLength]byte]*muChan
	}

	// DataColumnStorageOption is a functional option for configuring a DataColumnStorage.
	DataColumnStorageOption func(*DataColumnStorage) error

	// DataColumnsIdent is a collection of unique identifiers for data column sidecars.
	DataColumnsIdent struct {
		Root    [fieldparams.RootLength]byte
		Epoch   primitives.Epoch
		Indices []uint64
	}

	storageIndices struct {
		indices [mandatoryNumberOfColumns]byte
		count   int64
	}

	metadata struct {
		indices                         *storageIndices
		sszEncodedDataColumnSidecarSize uint32
		fileSize                        int64
	}

	fileMetadata struct {
		period    uint64
		epoch     primitives.Epoch
		blockRoot [fieldparams.RootLength]byte
	}

	muChan struct {
		mu      *sync.RWMutex
		toStore chan []blocks.VerifiedRODataColumn
	}
)

// DataColumnStorageReader is an interface to read data column sidecars from the filesystem.
type DataColumnStorageReader interface {
	Summary(root [fieldparams.RootLength]byte) DataColumnStorageSummary
	Get(root [fieldparams.RootLength]byte, indices []uint64) ([]blocks.VerifiedRODataColumn, error)
}

var _ DataColumnStorageReader = &DataColumnStorage{}

// WithDataColumnBasePath is a required option that sets the base path of data column storage.
func WithDataColumnBasePath(base string) DataColumnStorageOption {
	return func(b *DataColumnStorage) error {
		b.base = base
		return nil
	}
}

// WithDataColumnRetentionEpochs is an option that changes the number of epochs data columns will be persisted.
func WithDataColumnRetentionEpochs(e primitives.Epoch) DataColumnStorageOption {
	return func(b *DataColumnStorage) error {
		b.retentionEpochs = e
		return nil
	}
}

// WithDataColumnFs allows the afero.Fs implementation to be customized.
// Used by tests to substitute an in-memory filesystem.
func WithDataColumnFs(fs afero.Fs) DataColumnStorageOption {
	return func(b *DataColumnStorage) error {
		b.fs = fs
		return nil
	}
}

// NewDataColumnStorage creates a new instance of the DataColumnStorage object. Note that the implementation of DataColumnStorage may
// attempt to hold a file lock to guarantee exclusive control of the data column storage directory, so this should only be
// initialized once per beacon node.
func NewDataColumnStorage(ctx context.Context, opts ...DataColumnStorageOption) (*DataColumnStorage, error) {
	storage := &DataColumnStorage{
		dataColumnFeed: new(event.Feed),
		muChans:        make(map[[fieldparams.RootLength]byte]*muChan),
	}

	for _, o := range opts {
		if err := o(storage); err != nil {
			return nil, errors.Wrap(err, "failed to create data column storage")
		}
	}

	// Allow tests to set up a different fs using WithFs.
	if storage.fs == nil {
		if storage.base == "" {
			return nil, errNoDataColumnBasePath
		}

		storage.base = path.Clean(storage.base)
		if err := file.MkdirAll(storage.base); err != nil {
			return nil, errors.Wrapf(err, "failed to create data column storage at %s", storage.base)
		}

		storage.fs = afero.NewBasePathFs(afero.NewOsFs(), storage.base)
	}

	storage.cache = newDataColumnStorageSummaryCache()

	async.RunEvery(ctx, prunePeriod, func() {
		storage.pruneMu.Lock()
		defer storage.pruneMu.Unlock()

		storage.prune()
	})

	return storage, nil
}

// WarmCache warms the cache of the data column filesystem.
// It holds the database (read) lock for all the time it is running.
func (dcs *DataColumnStorage) WarmCache() {
	start := time.Now()
	log.Info("Data column filesystem cache warm-up started")

	dcs.pruneMu.Lock()
	defer dcs.pruneMu.Unlock()

	highestStoredEpoch := primitives.Epoch(0)

	// Walk the data column filesystem to warm up the cache.
	if err := afero.Walk(dcs.fs, ".", func(path string, info os.FileInfo, fileErr error) (err error) {
		if fileErr != nil {
			return fileErr
		}

		// If not a leaf, skip.
		if info.IsDir() {
			return nil
		}

		// Extract metadata from the file path.
		fileMetadata, err := extractFileMetadata(path)
		if err != nil {
			log.WithError(err).Error("Error encountered while extracting file metadata")
			return nil
		}

		// Open the data column filesystem file.
		f, err := dcs.fs.Open(path)
		if err != nil {
			log.WithError(err).Error("Error encountered while opening data column filesystem file")
			return nil
		}

		// Close the file.
		defer func() {
			// Overwrite the existing error only if it is nil, since the close error is less important.
			closeErr := f.Close()
			if closeErr != nil && err == nil {
				err = closeErr
			}
		}()

		// Read the metadata of the file.
		metadata, err := dcs.metadata(f)
		if err != nil {
			log.WithError(err).Error("Error encountered while reading metadata from data column filesystem file")
			return nil
		}

		// Check the indices.
		indices := metadata.indices.all()
		if len(indices) == 0 {
			return nil
		}

		// Build the ident.
		dataColumnsIdent := DataColumnsIdent{Root: fileMetadata.blockRoot, Epoch: fileMetadata.epoch, Indices: indices}

		// Update the highest stored epoch.
		highestStoredEpoch = max(highestStoredEpoch, fileMetadata.epoch)

		// Set the ident in the cache.
		if err := dcs.cache.set(dataColumnsIdent); err != nil {
			log.WithError(err).Error("Error encountered while ensuring data column filesystem cache")
		}

		return nil
	}); err != nil {
		log.WithError(err).Error("Error encountered while walking data column filesystem.")
	}

	// Prune the cache and the filesystem.
	dcs.prune()

	log.WithField("elapsed", time.Since(start)).Info("Data column filesystem cache warm-up complete")
}

// Summary returns the DataColumnStorageSummary.
func (dcs *DataColumnStorage) Summary(root [fieldparams.RootLength]byte) DataColumnStorageSummary {
	return dcs.cache.Summary(root)
}

// Save saves data column sidecars into the database and asynchronously performs pruning.
func (dcs *DataColumnStorage) Save(dataColumnSidecars []blocks.VerifiedRODataColumn) error {
	startTime := time.Now()

	if len(dataColumnSidecars) == 0 {
		return nil
	}

	// Check the number of columns is the one expected.
	// While implementing this, we expect the number of columns won't change.
	// If it does, we will need to create a new version of the data column sidecar file.
	if params.BeaconConfig().NumberOfColumns != mandatoryNumberOfColumns {
		return errWrongNumberOfColumns
	}

	dataColumnSidecarsByRoot := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn)

	// Group data column sidecars by root.
	for _, dataColumnSidecar := range dataColumnSidecars {
		// Check if the data column index is too large.
		if dataColumnSidecar.Index >= mandatoryNumberOfColumns {
			return errDataColumnIndexTooLarge
		}

		// Group data column sidecars by root.
		root := dataColumnSidecar.BlockRoot()
		dataColumnSidecarsByRoot[root] = append(dataColumnSidecarsByRoot[root], dataColumnSidecar)
	}

	for root, dataColumnSidecars := range dataColumnSidecarsByRoot {
		// Safety check all data column sidecars for this root are from the same slot.
		slot := dataColumnSidecars[0].Slot()
		for _, dataColumnSidecar := range dataColumnSidecars[1:] {
			if dataColumnSidecar.Slot() != slot {
				return errDataColumnSidecarsFromDifferentSlots
			}
		}

		// Save data columns in the filesystem.
		epoch := slots.ToEpoch(slot)
		if err := dcs.saveFilesystem(root, epoch, dataColumnSidecars); err != nil {
			return errors.Wrap(err, "save filesystem")
		}

		// Get all indices.
		indices := make([]uint64, 0, len(dataColumnSidecars))
		for _, dataColumnSidecar := range dataColumnSidecars {
			indices = append(indices, dataColumnSidecar.Index)
		}

		// Compute the data columns ident.
		dataColumnsIdent := DataColumnsIdent{Root: root, Epoch: epoch, Indices: indices}

		// Set data columns in the cache.
		if err := dcs.cache.set(dataColumnsIdent); err != nil {
			return errors.Wrap(err, "cache set")
		}

		// Notify the data column feed.
		dcs.dataColumnFeed.Send(dataColumnsIdent)
	}

	dataColumnSaveLatency.Observe(float64(time.Since(startTime).Milliseconds()))

	return nil
}

// Subscribe subscribes to the data column feed.
// It returns the subscription and a 1-size buffer channel to receive data column sidecars.
// It is the responsibility of the caller to:
// - call `subscription.Unsubscribe` when done, and to
// - read from the channel as fast as possible until the channel is closed.
// A call to `Save` will buffer a new value to the channel.
// If a call to `Save` is done while a value is already in the buffer of the channel:
// - the call to `Save` will block until the new value can be bufferized in the channel, and
// - all other subscribers won't be notified until the new value can be bufferized in the channel.
func (dcs *DataColumnStorage) Subscribe() (event.Subscription, <-chan DataColumnsIdent) {
	// Subscribe to newly data columns stored in the database.
	identsChan := make(chan DataColumnsIdent, 1)
	subscription := dcs.dataColumnFeed.Subscribe(identsChan)

	return subscription, identsChan
}

// saveFilesystem saves data column sidecars into the database.
// This function expects all data column sidecars to belong to the same block.
func (dcs *DataColumnStorage) saveFilesystem(root [fieldparams.RootLength]byte, epoch primitives.Epoch, dataColumnSidecars []blocks.VerifiedRODataColumn) error {
	// Compute the file path.
	filePath := filePath(root, epoch)

	dcs.pruneMu.RLock()
	defer dcs.pruneMu.RUnlock()

	fileMu, toStore := dcs.fileMutexChan(root)
	toStore <- dataColumnSidecars

	fileMu.Lock()
	defer fileMu.Unlock()

	// Check if the file exists.
	exists, err := afero.Exists(dcs.fs, filePath)
	if err != nil {
		return errors.Wrap(err, "afero exists")
	}

	if exists {
		if err := dcs.saveDataColumnSidecarsExistingFile(filePath, toStore); err != nil {
			return errors.Wrap(err, "save data column existing file")
		}

		return nil
	}

	if err := dcs.saveDataColumnSidecarsNewFile(filePath, toStore); err != nil {
		return errors.Wrap(err, "save data columns new file")
	}

	return nil
}

// Get retrieves data column sidecars from the database.
// If one of the requested data column sidecars is not found, it is just skipped.
// If indices is nil, then all stored data column sidecars are returned.
// Since DataColumnStorage only writes data columns that have undergone full verification, the return
// value is always a VerifiedRODataColumn.
func (dcs *DataColumnStorage) Get(root [fieldparams.RootLength]byte, indices []uint64) ([]blocks.VerifiedRODataColumn, error) {
	dcs.pruneMu.RLock()
	defer dcs.pruneMu.RUnlock()

	fileMu, _ := dcs.fileMutexChan(root)
	fileMu.RLock()
	defer fileMu.RUnlock()

	startTime := time.Now()

	// Build all indices if none are provided.
	if indices == nil {
		indices = make([]uint64, mandatoryNumberOfColumns)
		for i := range indices {
			indices[i] = uint64(i)
		}
	}

	summary, ok := dcs.cache.get(root)
	if !ok {
		// Nothing found in db. Exit early.
		return nil, nil
	}

	// Exit early if no data column sidecars for this root is stored.
	if !summary.HasAtLeastOneIndex(indices) {
		return nil, nil
	}

	// Compute the file path.
	filePath := filePath(root, summary.epoch)

	// Open the data column sidecars file.
	// We do not specially check if the file exists since we have already checked the cache.
	file, err := dcs.fs.Open(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "data column sidecars file path open")
	}

	// Read file metadata.
	metadata, err := dcs.metadata(file)
	if err != nil {
		return nil, errors.Wrap(err, "metadata")
	}

	// Retrieve data column sidecars from the file.
	verifiedRODataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, len(indices))
	for _, index := range indices {
		ok, position, err := metadata.indices.get(index)
		if err != nil {
			return nil, errors.Wrap(err, "get index")
		}

		// Skip if the data column is not saved.
		if !ok {
			continue
		}

		// Compute the offset of the data column sidecar.
		offset := headerSize + position*int64(metadata.sszEncodedDataColumnSidecarSize)

		// Seek to the beginning of the data column sidecar.
		_, err = file.Seek(offset, io.SeekStart)
		if err != nil {
			return nil, errors.Wrap(err, "seek")
		}

		verifiedRODataColumn, err := verification.VerifiedRODataColumnFromDisk(file, root, metadata.sszEncodedDataColumnSidecarSize)
		if err != nil {
			return nil, errors.Wrap(err, "verified RO data column from disk")
		}

		// Append the verified RO data column to the data column sidecars.
		verifiedRODataColumnSidecars = append(verifiedRODataColumnSidecars, verifiedRODataColumn)
	}

	dataColumnFetchLatency.Observe(float64(time.Since(startTime).Milliseconds()))

	return verifiedRODataColumnSidecars, nil
}

// Remove deletes all data column sidecars for a given root.
func (dcs *DataColumnStorage) Remove(blockRoot [fieldparams.RootLength]byte) error {
	dcs.pruneMu.RLock()
	defer dcs.pruneMu.RUnlock()

	fileMu, _ := dcs.fileMutexChan(blockRoot)
	fileMu.Lock()
	defer fileMu.Unlock()

	summary, ok := dcs.cache.get(blockRoot)
	if !ok {
		// Nothing found in db. Exit early.
		return nil
	}

	// Remove the data column sidecars from the cache.
	dcs.cache.evict(blockRoot)

	// Remove the data column sidecars file.
	filePath := filePath(blockRoot, summary.epoch)
	if err := dcs.fs.Remove(filePath); err != nil {
		return errors.Wrap(err, "remove")
	}

	return nil
}

// Clear deletes all files on the filesystem.
func (dcs *DataColumnStorage) Clear() error {
	dcs.pruneMu.Lock()
	defer dcs.pruneMu.Unlock()

	dirs, err := listDir(dcs.fs, ".")
	if err != nil {
		return errors.Wrap(err, "list dir")
	}

	dcs.cache.clear()

	for _, dir := range dirs {
		if err := dcs.fs.RemoveAll(dir); err != nil {
			return errors.Wrap(err, "remove all")
		}
	}

	return nil
}

// prune clean the cache, the filesystem and mutexes.
func (dcs *DataColumnStorage) prune() {
	highestStoredEpoch := dcs.cache.HighestEpoch()

	// Check if we need to prune.
	if highestStoredEpoch < dcs.retentionEpochs {
		return
	}

	highestEpochToPrune := highestStoredEpoch - dcs.retentionEpochs
	highestPeriodToPrune := period(highestEpochToPrune)

	// Prune the cache.
	prunedCount := dcs.cache.pruneUpTo(highestEpochToPrune)

	if prunedCount == 0 {
		return
	}

	dataColumnPrunedCounter.Add(float64(prunedCount))

	// Prune the filesystem.
	periodFileInfos, err := afero.ReadDir(dcs.fs, ".")
	if err != nil {
		log.WithError(err).Error("Error encountered while reading top directory")
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
			// Remove everything lower thant highest period to prune.
			if err := dcs.fs.RemoveAll(periodStr); err != nil {
				log.WithError(err).Error("Error encountered while removing period directory")
			}

			continue
		}

		if period > highestPeriodToPrune {
			// Do not remove anything higher than highest period to prune.
			continue
		}

		// if period == highestPeriodToPrune
		epochFileInfos, err := afero.ReadDir(dcs.fs, periodStr)
		if err != nil {
			log.WithError(err).Error("Error encountered while reading epoch directory")
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

			if err := dcs.fs.RemoveAll(epochDir); err != nil {
				log.WithError(err).Error("Error encountered while removing epoch directory")
				continue
			}
		}
	}

	dcs.mu.Lock()
	defer dcs.mu.Unlock()
	clear(dcs.muChans)
}

// saveDataColumnSidecarsExistingFile saves data column sidecars into an existing file.
// This function expects all data column sidecars to belong to the same block.
func (dcs *DataColumnStorage) saveDataColumnSidecarsExistingFile(filePath string, inputDataColumnSidecars chan []blocks.VerifiedRODataColumn) (err error) {
	// Open the data column sidecars file.
	file, err := dcs.fs.OpenFile(filePath, os.O_RDWR, os.FileMode(0600))
	if err != nil {
		return errors.Wrap(err, "data column sidecars file path open")
	}

	defer func() {
		closeErr := file.Close()

		// Overwrite the existing error only if it is nil, since the close error is less important.
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	metadata, err := dcs.metadata(file)
	if err != nil {
		return errors.Wrap(err, "metadata")
	}

	// Create the SSZ encoded data column sidecars.
	var sszEncodedDataColumnSidecars []byte

	for {
		dataColumnSidecars := pullChan(inputDataColumnSidecars)
		if len(dataColumnSidecars) == 0 {
			break
		}

		for _, dataColumnSidecar := range dataColumnSidecars {
			// Extract the data columns index.
			dataColumnIndex := dataColumnSidecar.Index

			ok, _, err := metadata.indices.get(dataColumnIndex)
			if err != nil {
				return errors.Wrap(err, "get index")
			}

			// Skip if the data column is already saved.
			if ok {
				continue
			}

			// Check if the number of saved data columns is too large.
			// This is impossible to happen in practice is this function is called
			// by SaveDataColumnSidecars.
			if metadata.indices.len() >= mandatoryNumberOfColumns {
				return errTooManyDataColumns
			}

			// SSZ encode the data column sidecar.
			sszEncodedDataColumnSidecar, err := dataColumnSidecar.MarshalSSZ()
			if err != nil {
				return errors.Wrap(err, "data column sidecar marshal SSZ")
			}

			// Compute the size of the SSZ encoded data column sidecar.
			incomingSszEncodedDataColumnSidecarSize := uint32(len(sszEncodedDataColumnSidecar))

			// Check if the incoming encoded data column sidecar size corresponds to the one read from the file.
			if incomingSszEncodedDataColumnSidecarSize != metadata.sszEncodedDataColumnSidecarSize {
				return errWrongSszEncodedDataColumnSidecarSize
			}

			// Alter indices to mark the data column as saved.
			if err := metadata.indices.set(dataColumnIndex, uint8(metadata.indices.len())); err != nil {
				return errors.Wrap(err, "set index")
			}

			// Append the SSZ encoded data column sidecar to the SSZ encoded data column sidecars.
			sszEncodedDataColumnSidecars = append(sszEncodedDataColumnSidecars, sszEncodedDataColumnSidecar...)
		}
	}

	// Save indices to the file.
	indices := metadata.indices.raw()
	count, err := file.WriteAt(indices[:], int64(versionSize+sidecarByteLenSize))
	if err != nil {
		return errors.Wrap(err, "write indices")
	}
	if count != mandatoryNumberOfColumns {
		return errWrongBytesWritten
	}

	// Append the SSZ encoded data column sidecars to the end of the file.
	count, err = file.WriteAt(sszEncodedDataColumnSidecars, metadata.fileSize)
	if err != nil {
		return errors.Wrap(err, "write SSZ encoded data column sidecars")
	}
	if count != len(sszEncodedDataColumnSidecars) {
		return errWrongBytesWritten
	}

	if err := file.Sync(); err != nil {
		return errors.Wrap(err, "sync")
	}

	return nil
}

// saveDataColumnSidecarsNewFile saves data column sidecars into a new file.
// This function expects all data column sidecars to belong to the same block.
func (dcs *DataColumnStorage) saveDataColumnSidecarsNewFile(filePath string, inputDataColumnSidecars chan []blocks.VerifiedRODataColumn) (err error) {
	// Initialize the indices.
	var indices storageIndices

	var (
		sszEncodedDataColumnSidecarRefSize int
		sszEncodedDataColumnSidecars       []byte
	)

	// Initialize the count of the saved SSZ encoded data column sidecar.
	storedCount := uint8(0)

	for {
		dataColumnSidecars := pullChan(inputDataColumnSidecars)
		if len(dataColumnSidecars) == 0 {
			break
		}

		for _, dataColumnSidecar := range dataColumnSidecars {
			// Extract the data column index.
			dataColumnIndex := dataColumnSidecar.Index

			// Skip if the data column is already stored.
			ok, _, err := indices.get(dataColumnIndex)
			if err != nil {
				return errors.Wrap(err, "get index")
			}
			if ok {
				continue
			}

			// Alter the indices to mark the first data column sidecar as saved.
			// savedCount can safely be cast to uint8 since it is less than limit.
			if err := indices.set(dataColumnIndex, storedCount); err != nil {
				return errors.Wrap(err, "set index")
			}

			// Increment the count of the saved SSZ encoded data column sidecar.
			storedCount++

			// SSZ encode the first data column sidecar.
			sszEncodedDataColumnSidecar, err := dataColumnSidecar.MarshalSSZ()
			if err != nil {
				return errors.Wrap(err, "data column sidecar marshal SSZ")
			}

			// Check if the size of the SSZ encoded data column sidecar is correct.
			if sszEncodedDataColumnSidecarRefSize != 0 && len(sszEncodedDataColumnSidecar) != sszEncodedDataColumnSidecarRefSize {
				return errWrongSszEncodedDataColumnSidecarSize
			}

			// Set the SSZ encoded data column sidecar reference size.
			sszEncodedDataColumnSidecarRefSize = len(sszEncodedDataColumnSidecar)

			// Append the first SSZ encoded data column sidecar to the SSZ encoded data column sidecars.
			sszEncodedDataColumnSidecars = append(sszEncodedDataColumnSidecars, sszEncodedDataColumnSidecar...)
		}
	}

	if storedCount == 0 {
		// Nothing to save.
		return nil
	}

	// Create the data column sidecars file.
	dir := filepath.Dir(filePath)
	if err := dcs.fs.MkdirAll(dir, directoryPermissions()); err != nil {
		return errors.Wrapf(err, "mkdir all")
	}

	file, err := dcs.fs.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "data column sidecars file path create")
	}

	defer func() {
		closeErr := file.Close()

		// Overwrite the existing error only if it is nil, since the close error is less important.
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	// Encode the SSZ encoded data column sidecar size.
	var encodedSszEncodedDataColumnSidecarSize [sidecarByteLenSize]byte
	binary.BigEndian.PutUint32(encodedSszEncodedDataColumnSidecarSize[:], uint32(sszEncodedDataColumnSidecarRefSize))

	// Get the raw indices.
	rawIndices := indices.raw()

	// Concatenate the version, the data column sidecar size, the data column indices and the SSZ encoded data column sidecar.
	countToWrite := headerSize + len(sszEncodedDataColumnSidecars)
	bytes := make([]byte, 0, countToWrite)
	bytes = append(bytes, byte(version))
	bytes = append(bytes, encodedSszEncodedDataColumnSidecarSize[:]...)
	bytes = append(bytes, rawIndices[:]...)
	bytes = append(bytes, sszEncodedDataColumnSidecars...)

	countWritten, err := file.Write(bytes)
	if err != nil {
		return errors.Wrap(err, "write")
	}
	if countWritten != countToWrite {
		return errWrongBytesWritten
	}

	if err := file.Sync(); err != nil {
		return errors.Wrap(err, "sync")
	}

	return nil
}

// metadata runs file sanity checks and retrieves metadata of the file.
// The file descriptor is left at the beginning of the first SSZ encoded data column sidecar.
func (dcs *DataColumnStorage) metadata(file afero.File) (*metadata, error) {
	var header [headerSize]byte
	countRead, err := file.ReadAt(header[:], 0)
	if err != nil {
		return nil, errors.Wrap(err, "read at")
	}
	if countRead != headerSize {
		return nil, errWrongBytesHeaderRead
	}

	// Read the encoded file version.
	encodedFileVersion := header[versionOffset : versionOffset+versionSize]

	// Convert the version to an int.
	fileVersion := int(encodedFileVersion[0])

	// Check if the version is the expected one.
	if fileVersion != version {
		return nil, errWrongVersion
	}

	// DataColumnSidecar is a variable sized ssz object, but all data columns for a block will be the same size.
	encodedSszEncodedDataColumnSidecarSize := header[sidecarByteLenOffset : sidecarByteLenOffset+sidecarByteLenSize]

	// Convert the SSZ encoded data column sidecar size to an int.
	sszEncodedDataColumnSidecarSize := binary.BigEndian.Uint32(encodedSszEncodedDataColumnSidecarSize)

	// Read the data column indices.
	indices, err := newStorageIndices(header[indicesOffset : indicesOffset+mandatoryNumberOfColumns])
	if err != nil {
		return nil, errors.Wrap(err, "new storage indices")
	}

	// Compute the saved columns count.
	savedDataColumnSidecarCount := indices.len()

	// Compute the size of the file.
	// It is safe to cast the SSZ encoded data column sidecar size to int64 since it is less than 2**63.
	fileSize := int64(headerSize) + savedDataColumnSidecarCount*int64(sszEncodedDataColumnSidecarSize) // lint:ignore uintcast

	metadata := &metadata{
		indices:                         indices,
		sszEncodedDataColumnSidecarSize: sszEncodedDataColumnSidecarSize,
		fileSize:                        fileSize,
	}

	return metadata, nil
}

func (dcs *DataColumnStorage) fileMutexChan(root [fieldparams.RootLength]byte) (*sync.RWMutex, chan []blocks.VerifiedRODataColumn) {
	dcs.mu.Lock()
	defer dcs.mu.Unlock()

	mc, ok := dcs.muChans[root]
	if !ok {
		mc = &muChan{
			mu:      new(sync.RWMutex),
			toStore: make(chan []blocks.VerifiedRODataColumn, 1),
		}
		dcs.muChans[root] = mc

		return mc.mu, mc.toStore
	}

	return mc.mu, mc.toStore
}

func newStorageIndices(originalIndices []byte) (*storageIndices, error) {
	if len(originalIndices) != mandatoryNumberOfColumns {
		return nil, errWrongNumberOfColumns
	}

	count := int64(0)
	for _, i := range originalIndices {
		if i >= nonZeroOffset {
			count++
		}
	}

	var indices [mandatoryNumberOfColumns]byte
	copy(indices[:], originalIndices)

	storageIndices := storageIndices{
		indices: indices,
		count:   count,
	}

	return &storageIndices, nil
}

// get returns a boolean indicating if the data column sidecar is saved,
// and the position of the data column sidecar in the file.
func (si *storageIndices) get(dataColumnIndex uint64) (bool, int64, error) {
	if dataColumnIndex >= mandatoryNumberOfColumns {
		return false, 0, errDataColumnIndexTooLarge
	}

	if si.indices[dataColumnIndex] < nonZeroOffset {
		return false, 0, nil
	}

	return true, int64(si.indices[dataColumnIndex] - nonZeroOffset), nil
}

func (si *storageIndices) len() int64 {
	return si.count
}

// all returns all saved data column sidecars.
func (si *storageIndices) all() []uint64 {
	indices := make([]uint64, 0, len(si.indices))

	for index, i := range si.indices {
		if i >= nonZeroOffset {
			indices = append(indices, uint64(index))
		}
	}

	return indices
}

// raw returns the raw data column sidecar indices.
// It can be safely modified by the caller.
func (si *storageIndices) raw() [mandatoryNumberOfColumns]byte {
	var result [mandatoryNumberOfColumns]byte
	copy(result[:], si.indices[:])
	return result
}

// set sets the data column sidecar as saved.
func (si *storageIndices) set(dataColumnIndex uint64, position uint8) error {
	if dataColumnIndex >= mandatoryNumberOfColumns || position >= mandatoryNumberOfColumns {
		return errDataColumnIndexTooLarge
	}

	existing := si.indices[dataColumnIndex] >= nonZeroOffset
	if !existing {
		si.count++
	}

	si.indices[dataColumnIndex] = nonZeroOffset + position

	return nil
}

// pullChan pulls data column sidecars from the input channel until it is empty.
func pullChan(inputRoDataColumns chan []blocks.VerifiedRODataColumn) []blocks.VerifiedRODataColumn {
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	dataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, numberOfColumns)

	for {
		select {
		case dataColumnSidecar := <-inputRoDataColumns:
			dataColumnSidecars = append(dataColumnSidecars, dataColumnSidecar...)
		default:
			return dataColumnSidecars
		}
	}
}

// filePath builds the file path in database for a given root and epoch.
func filePath(root [fieldparams.RootLength]byte, epoch primitives.Epoch) string {
	return path.Join(
		fmt.Sprintf("%d", period(epoch)),
		fmt.Sprintf("%d", epoch),
		fmt.Sprintf("%#x.%s", root, dataColumnsFileExtension),
	)
}

// extractFileMetadata extracts the metadata from a file path.
// If the path is not a leaf, it returns nil.
func extractFileMetadata(path string) (*fileMetadata, error) {
	// Use filepath.Separator to handle both Windows (\) and Unix (/) path separators
	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) != 3 {
		return nil, errors.Errorf("unexpected file %s", path)
	}

	period, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse period from %s", path)
	}

	epoch, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse epoch from %s", path)
	}

	partsRoot := strings.Split(parts[2], ".")
	if len(partsRoot) != 2 {
		return nil, errors.Errorf("failed to parse root from %s", path)
	}

	blockRootString := partsRoot[0]
	if len(blockRootString) != 2+2*fieldparams.RootLength || blockRootString[:2] != "0x" {
		return nil, errors.Errorf("unexpected file name %s", path)
	}

	if partsRoot[1] != dataColumnsFileExtension {
		return nil, errors.Errorf("unexpected extension %s", path)
	}

	blockRootSlice, err := hex.DecodeString(blockRootString[2:])
	if err != nil {
		return nil, errors.Wrapf(err, "decode string from %s", path)
	}

	var blockRoot [fieldparams.RootLength]byte
	copy(blockRoot[:], blockRootSlice)

	result := &fileMetadata{period: period, epoch: primitives.Epoch(epoch), blockRoot: blockRoot}
	return result, nil
}

// period computes the period of a given epoch.
func period(epoch primitives.Epoch) uint64 {
	return uint64(epoch / params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)
}
