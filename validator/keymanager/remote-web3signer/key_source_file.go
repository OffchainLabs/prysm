package remote_web3signer

import (
	"bufio"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
		km.updatePublicKeys(slices.Collect(maps.Values(km.flagLoadedKeysMap))) // update the keys to flag provided defaults
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

func (km *Keymanager) readKeyFile() ([][48]byte, map[string][48]byte, error) {
	km.lock.RLock()
	defer km.lock.RUnlock()

	if km.keyFilePath == "" {
		return nil, nil, errors.New("no key file path provided")
	}
	f, err := os.Open(filepath.Clean(km.keyFilePath))
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not open remote signer public key file")
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.WithError(err).Error("Could not close remote signer public key file")
		}
	}()
	// Use a map to track and skip duplicate lines
	seenKeys := make(map[string][48]byte)
	scanner := bufio.NewScanner(f)
	var keys [][48]byte
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
			pubkey, err := hexutil.Decode(line)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "could not decode public key %s in remote signer key file", line)
			}
			bPubkey := bytesutil.ToBytes48(pubkey)
			seenKeys[line] = bPubkey
			keys = append(keys, bPubkey)
		}
	}
	// Check for scanning errors
	if err := scanner.Err(); err != nil {
		return nil, nil, errors.Wrap(err, "could not scan remote signer public key file")
	}
	if len(keys) == 0 {
		log.Warn("Remote signer key file: no valid public keys found. Defaulting to flag provided keys if any exist.")
	}
	return keys, seenKeys, nil
}

func (km *Keymanager) savePublicKeysToFile(providedPublicKeys map[string][48]byte) error {
	if km.keyFilePath == "" {
		return errors.New("no key file provided")
	}
	pubkeys := make([][48]byte, 0)
	// Open the file with write and truncate permissions
	f, err := os.OpenFile(km.keyFilePath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.WithError(err).Error("Could not close file, proceeding without closing the file")
		}
	}(f)

	// Iterate through all lines in the slice and write them to the file
	for key, value := range providedPublicKeys {
		if _, err := f.WriteString(key + "\n"); err != nil {
			return fmt.Errorf("error writing key %s to file: %w", value, err)
		}
		pubkeys = append(pubkeys, value)
	}
	km.updatePublicKeys(pubkeys)
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
	fileKeys, _, err := km.readKeyFile()
	if err != nil {
		return errors.Wrap(err, "could not read key file")
	}
	if len(fileKeys) == 0 {
		log.Warnln("Remote signer key file no longer has keys, defaulting to flag provided keys")
		fileKeys = slices.Collect(maps.Values(km.flagLoadedKeysMap))
	}
	km.updatePublicKeys(fileKeys)
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
				fileKeys, _, err := km.readKeyFile()
				if err != nil {
					return errors.New("could not read key file")
				}
				// prioritize file keys over flag keys
				if len(fileKeys) == 0 {
					log.Warnln("Remote signer key file no longer has keys, defaulting to flag provided keys")
					fileKeys = slices.Collect(maps.Values(km.flagLoadedKeysMap))
				}
				km.updatePublicKeys(fileKeys)
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
