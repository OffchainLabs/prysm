package remote_web3signer

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (km *Keymanager) refreshRemoteKeysFromFileChangesWithRetry(ctx context.Context, retryDelay time.Duration, markReady func(error)) error {
	initialized := false
	markInitialized := func(err error) {
		initialized = true
		markReady(err)
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if km.retriesRemaining == 0 {
			return errors.New("file check retries remaining exceeded")
		}
		err := km.refreshRemoteKeysFromFileChanges(ctx, markInitialized)
		if err == nil {
			return nil
		}
		// Retries only make sense once the watcher has worked at least once.
		// Before that, fail startup immediately instead of blocking NewKeymanager
		// through the retry ladder.
		if !initialized {
			return err
		}

		// drop the all file keys, the union falls back to the flag and URL keys
		km.replaceKeys(sourceFile, nil)

		km.retriesRemaining--
		log.WithError(err).Debug("Error occurred on key refresh")
		log.WithFields(logrus.Fields{"path": km.keyFilePath, "retriesRemaining": km.retriesRemaining, "retryDelay": retryDelay}).Warnf("Could not refresh keys. Retrying...")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
		}
	}
}

// reloadKeyFile re-reads the key file and makes its contents the file source's set.
// updateLock covers the read and the replace together, so a keymanager API write cannot
// interleave and leave the file source holding a truncated or stale set.
func (km *Keymanager) reloadKeyFile() error {
	km.updateLock.Lock()
	defer km.updateLock.Unlock()

	fileKeys, err := km.readKeyFile()
	if err != nil {
		return fmt.Errorf("read key file: %w", err)
	}
	if len(fileKeys) == 0 {
		log.Warnln("Remote signer key file has no keys, so the key file no longer contributes any validating keys. Keys from the flag and the public keys URL are unaffected")
	}
	km.replaceKeysLocked(sourceFile, fileKeys)
	return nil
}

func (km *Keymanager) readKeyFile() ([]pubkey, error) {
	if km.keyFilePath == "" {
		return nil, errors.New("no key file path provided")
	}
	f, err := os.Open(filepath.Clean(km.keyFilePath))
	if err != nil {
		return nil, errors.Wrap(err, "could not open remote signer public key file")
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.WithError(err).Error("Could not close remote signer public key file")
		}
	}()
	// Use a map to track and skip duplicate lines
	seenKeys := make(map[string]struct{})
	scanner := bufio.NewScanner(f)
	var keys []pubkey
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		pubkeyLength := (fieldparams.BLSPubkeyLength * 2) + 2
		if line == "" {
			// skip empty line
			continue
		}
		// allow for pubkeys without the 0x
		if len(line) == pubkeyLength-2 && !strings.HasPrefix(line, "0x") {
			line = "0x" + line
		}
		if len(line) != pubkeyLength {
			log.WithFields(logrus.Fields{
				"filepath": km.keyFilePath,
				"key":      line,
			}).Error("Invalid public key in remote signer key file")
			continue
		}
		if _, found := seenKeys[line]; !found {
			// If it's a new line, mark it as seen and process it
			key, err := bytesutil.DecodeHex48(line)
			if err != nil {
				return nil, errors.Wrapf(err, "could not decode public key %s in remote signer key file", line)
			}
			seenKeys[line] = struct{}{}
			keys = append(keys, key)
		}
	}
	// Check for scanning errors
	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "could not scan remote signer public key file")
	}
	return keys, nil
}

// savePublicKeysToFile writes keys to the key file and makes them the file source's set.
// The caller must hold updateLock so the watcher cannot read a half-written file.
func (km *Keymanager) savePublicKeysToFile(keys []pubkey) error {
	if km.keyFilePath == "" {
		return errors.New("no key file provided")
	}
	// Open the file with write and truncate permissions
	f, err := os.OpenFile(km.keyFilePath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	// The keymanager API reports these keys as stored permanently, so flush and close before
	// claiming so rather than discovering a write error after the response has gone out.
	err = writePublicKeys(f, keys)
	if syncErr := f.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write remote signer key file: %w", err)
	}

	km.replaceKeysLocked(sourceFile, keys)
	return nil
}

func (km *Keymanager) refreshRemoteKeysFromFileChanges(ctx context.Context, markReady func(error)) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "could not initialize file watcher")
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			log.WithError(err).Error("Could not close file watcher")
		}
	}()
	initialFileInfo, err := os.Stat(km.keyFilePath)
	if err != nil {
		return errors.Wrap(err, "could not stat remote signer public key file")
	}
	initialFileSize := initialFileInfo.Size()
	if err := watcher.Add(km.keyFilePath); err != nil {
		return errors.Wrap(err, "could not add file to file watcher")
	}
	log.WithField("path", km.keyFilePath).Info("Successfully initialized file watcher")
	km.retriesRemaining = maxRetries // reset retries to default
	// Reload keys on every watcher (re)initialization: a failed refresh resets the
	// in-memory keys to flag-provided defaults, so a recovery must restore the
	// file-loaded set even when the flag-provided keys are not empty.
	if err := km.reloadKeyFile(); err != nil {
		return fmt.Errorf("reload key file: %w", err)
	}
	markReady(nil)
	for {
		select {
		case e, ok := <-watcher.Events:
			if !ok { // Channel was closed (i.e. Watcher.Close() was called).
				log.Info("Closing file watcher")
				return nil
			}
			log.WithFields(logrus.Fields{
				"event": e.Name,
				"op":    e.Op.String(),
			}).Debug("Remote signer key file event triggered")
			if e.Has(fsnotify.Remove) {
				return errors.New("remote signer key file was removed")
			}
			currentFileInfo, err := os.Stat(km.keyFilePath)
			if err != nil {
				return errors.Wrap(err, "could not stat remote signer public key file")
			}
			if currentFileInfo.Size() != initialFileSize {
				log.Info("Remote signer key file updated")
				if err := km.reloadKeyFile(); err != nil {
					return fmt.Errorf("reload key file: %w", err)
				}
				initialFileSize = currentFileInfo.Size()
			}
		case err, ok := <-watcher.Errors:
			if !ok { // Channel was closed (i.e. Watcher.Close() was called).
				log.Info("Closing file watcher")
				return nil
			}
			return errors.Wrap(err, "could not watch for file changes")
		case <-ctx.Done():
			log.Info("Closing file watcher")
			return nil
		}
	}
}

func writePublicKeys(f *os.File, keys []pubkey) error {
	written := make(map[pubkey]struct{}, len(keys))
	for _, key := range keys {
		if _, ok := written[key]; ok {
			continue
		}
		written[key] = struct{}{}
		if _, err := f.WriteString(hexutil.Encode(key[:]) + "\n"); err != nil {
			return fmt.Errorf("error writing key %#x to file: %w", key, err)
		}
	}
	return nil
}
