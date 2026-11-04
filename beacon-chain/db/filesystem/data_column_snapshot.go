package filesystem

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"runtime"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/spf13/afero"
)

// The data column cache snapshot is a dump of the data column summary cache, written at graceful shutdown
// and consumed (then deleted) at the next startup.
// Its presence means the previous run shut down gracefully.
const (
	snapshotFileName        = "cache.snapshot"          // Snapshot file, at the root of the data column storage
	snapshotTmpFileName     = snapshotFileName + ".tmp" // Snapshot being written, renamed to snapshotFileName once complete
	snapshotFilePermissions = os.FileMode(0600)         // Read/write for the owner only
	snapshotMagic           = "PRYSMDCS"                // Prysm Data Column Snapshot, identifies the file
	snapshotFormatVersion   = uint16(1)                 // Version of the file structure
	bitsPerByte             = 8                         // Number of bits in a byte
	goosWindows             = "windows"                 // Value of runtime.GOOS on Windows

	// Header
	snapshotVersionSize   = 2                                           // Format version (uint16)
	snapshotCountSize     = 8                                           // Entries count (uint64)
	snapshotVersionOffset = len(snapshotMagic)                          // Format version, right after the magic
	snapshotCountOffset   = snapshotVersionOffset + snapshotVersionSize // Entries count, right after the format version
	snapshotHeaderSize    = snapshotCountOffset + snapshotCountSize     // Magic + format version + entries count = 18 bytes

	// Entry
	snapshotEpochSize   = 8                                       // Epoch (uint64)
	snapshotMaskSize    = mandatoryNumberOfColumns / bitsPerByte  // Stored indices bitfield, one bit per column = 16 bytes
	snapshotEpochOffset = fieldparams.RootLength                  // Epoch, right after the block root
	snapshotMaskOffset  = snapshotEpochOffset + snapshotEpochSize // Stored indices bitfield, right after the epoch
	snapshotEntrySize   = snapshotMaskOffset + snapshotMaskSize   // Block root + epoch + stored indices bitfield = 56 bytes

	// Trailer
	snapshotTrailerSize = 4 // CRC32 (uint32)
)

var (
	errSnapshotWrongMagic    = errors.New("wrong snapshot magic")
	errSnapshotWrongVersion  = errors.New("wrong snapshot format version")
	errSnapshotWrongSize     = errors.New("wrong snapshot size")
	errSnapshotWrongChecksum = errors.New("wrong snapshot checksum")
)

// writeSnapshot atomically and durably writes the content of the cache to the snapshot file.
// The snapshot is written to a temporary file, synced, then renamed, so the snapshot file is either absent or complete.
func (dcs *DataColumnStorage) writeSnapshot() error {
	if err := dcs.writeSnapshotTmp(); err != nil {
		if removeErr := dcs.fs.Remove(snapshotTmpFileName); removeErr != nil && !os.IsNotExist(removeErr) {
			log.WithError(removeErr).Warning("Could not remove temporary data column cache snapshot")
		}

		return fmt.Errorf("write temporary snapshot: %w", err)
	}

	if err := dcs.fs.Rename(snapshotTmpFileName, snapshotFileName); err != nil {
		return fmt.Errorf("rename snapshot: %w", err)
	}

	if err := syncDir(dcs.fs, "."); err != nil {
		log.WithError(err).Warning("Could not sync data column storage directory after writing cache snapshot")
	}

	return nil
}

func (dcs *DataColumnStorage) writeSnapshotTmp() (err error) {
	file, err := dcs.fs.OpenFile(snapshotTmpFileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, snapshotFilePermissions)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}

	defer func() {
		closeErr := file.Close()

		// Overwrite the existing error only if it is nil, since the close error is less important.
		if closeErr != nil && err == nil {
			err = fmt.Errorf("close: %w", closeErr)
		}
	}()

	checksum := crc32.NewIEEE()
	writer := bufio.NewWriter(io.MultiWriter(file, checksum))

	if err := dcs.cache.writeSnapshot(writer); err != nil {
		return fmt.Errorf("write entries: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}

	if err := binary.Write(file, binary.LittleEndian, checksum.Sum32()); err != nil {
		return fmt.Errorf("write checksum: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	return nil
}

// consumeSnapshot loads the snapshot file, if any, into the cache, then deletes it.
// It returns true if the cache was loaded from the snapshot.
// If it returns false, the cache is left empty.
func (dcs *DataColumnStorage) consumeSnapshot() bool {
	// A leftover temporary snapshot means the previous run was killed while writing the snapshot.
	if err := dcs.fs.Remove(snapshotTmpFileName); err != nil && !os.IsNotExist(err) {
		log.WithError(err).Warning("Could not remove temporary data column cache snapshot")
	}

	exists, err := afero.Exists(dcs.fs, snapshotFileName)
	if err != nil {
		log.WithError(err).Warning("Could not check data column cache snapshot existence")
		return false
	}

	if !exists {
		log.
			WithField("path", filepath.Join(dcs.base, snapshotFileName)).
			Info("No data column cache snapshot found (first start or previous shutdown was not graceful)")
		return false
	}

	loadErr := dcs.loadSnapshot()

	// The snapshot must be removed before the node writes anything, otherwise a crash later in this run
	// would leave a stale snapshot behind, trusted at the next startup.
	if err := dcs.fs.Remove(snapshotFileName); err != nil {
		log.WithError(err).Error("Could not remove data column cache snapshot")
		dcs.cache.clear()
		return false
	}

	if err := syncDir(dcs.fs, "."); err != nil {
		log.WithError(err).Error("Could not sync data column storage directory after removing cache snapshot")
		dcs.cache.clear()
		return false
	}

	if loadErr != nil {
		log.WithError(loadErr).Warning("Could not load data column cache snapshot")
		dcs.cache.clear()
		return false
	}

	return true
}

func (dcs *DataColumnStorage) loadSnapshot() (err error) {
	info, err := dcs.fs.Stat(snapshotFileName)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}

	file, err := dcs.fs.Open(snapshotFileName)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.WithError(closeErr).Warning("Could not close data column cache snapshot")
		}
	}()

	checksum := crc32.NewIEEE()
	reader := io.TeeReader(bufio.NewReader(file), checksum)

	if err := dcs.cache.readSnapshot(reader, info.Size()); err != nil {
		return fmt.Errorf("read entries: %w", err)
	}

	computedChecksum := checksum.Sum32()

	var storedChecksum uint32
	if err := binary.Read(reader, binary.LittleEndian, &storedChecksum); err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}

	if storedChecksum != computedChecksum {
		return errSnapshotWrongChecksum
	}

	return nil
}

// writeSnapshot writes the header and all the entries of the cache.
func (sc *dataColumnStorageSummaryCache) writeSnapshot(writer io.Writer) error {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	header := make([]byte, 0, snapshotHeaderSize)
	header = append(header, snapshotMagic...)
	header = binary.LittleEndian.AppendUint16(header, snapshotFormatVersion)
	header = binary.LittleEndian.AppendUint64(header, uint64(len(sc.cache)))

	if _, err := writer.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	var entry [snapshotEntrySize]byte
	for root, summary := range sc.cache {
		copy(entry[:snapshotEpochOffset], root[:])
		binary.LittleEndian.PutUint64(entry[snapshotEpochOffset:], uint64(summary.epoch))

		mask := entry[snapshotMaskOffset:]
		clear(mask)
		for index, stored := range summary.mask {
			if stored {
				mask[index/bitsPerByte] |= 1 << (index % bitsPerByte)
			}
		}

		if _, err := writer.Write(entry[:]); err != nil {
			return fmt.Errorf("write entry: %w", err)
		}
	}

	return nil
}

// readSnapshot reads the header and all the entries of a snapshot of size `size` into the cache.
func (sc *dataColumnStorageSummaryCache) readSnapshot(reader io.Reader, size int64) error {
	var header [snapshotHeaderSize]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return fmt.Errorf("read header: %w", err)
	}

	if string(header[:snapshotVersionOffset]) != snapshotMagic {
		return errSnapshotWrongMagic
	}

	if binary.LittleEndian.Uint16(header[snapshotVersionOffset:]) != snapshotFormatVersion {
		return errSnapshotWrongVersion
	}

	// Check the size before reading entries, so a corrupted count cannot trigger a huge read.
	count := binary.LittleEndian.Uint64(header[snapshotCountOffset:])
	expectedSize := uint64(snapshotHeaderSize) + count*snapshotEntrySize + snapshotTrailerSize
	if size < 0 || count > uint64(size)/snapshotEntrySize || expectedSize != uint64(size) {
		return errSnapshotWrongSize
	}

	var entry [snapshotEntrySize]byte
	indices := make([]uint64, 0, mandatoryNumberOfColumns)
	for range count {
		if _, err := io.ReadFull(reader, entry[:]); err != nil {
			return fmt.Errorf("read entry: %w", err)
		}

		var root [fieldparams.RootLength]byte
		copy(root[:], entry[:snapshotEpochOffset])
		epoch := primitives.Epoch(binary.LittleEndian.Uint64(entry[snapshotEpochOffset:]))

		indices = indices[:0]
		mask := entry[snapshotMaskOffset:]
		for index := range uint64(mandatoryNumberOfColumns) {
			if mask[index/bitsPerByte]&(1<<(index%bitsPerByte)) != 0 {
				indices = append(indices, index)
			}
		}

		if err := sc.set(DataColumnsIdent{Root: root, Epoch: epoch, Indices: indices}); err != nil {
			return fmt.Errorf("cache set: %w", err)
		}
	}

	return nil
}

// syncDir makes durable the changes (creations, renames, removals) done on the entries of the directory.
// Windows does not support syncing directories, and NTFS journals metadata changes itself.
func syncDir(fs afero.Fs, dir string) (err error) {
	if runtime.GOOS == goosWindows {
		return nil
	}

	file, err := fs.Open(dir)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}

	defer func() {
		closeErr := file.Close()

		// Overwrite the existing error only if it is nil, since the close error is less important.
		if closeErr != nil && err == nil {
			err = fmt.Errorf("close: %w", closeErr)
		}
	}()

	return file.Sync()
}
