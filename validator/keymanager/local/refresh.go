package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	keystorev4 "github.com/wealdtech/go-eth2-wallet-encryptor-keystorev4"
)

// Listen for changes to the all-accounts.keystore.json file in our wallet
// to load in new keys we observe into our keymanager. This uses the fsnotify
// library to listen for file-system changes and debounces these events to
// ensure we can handle thousands of events fired in a short time-span.
func (km *Keymanager) listenForAccountChanges(ctx context.Context) {
	debounceFileChangesInterval := features.Get().KeystoreImportDebounceInterval
	accountsFilePath := filepath.Join(km.wallet.AccountsDir(), AccountsPath, AccountsKeystoreFileName)
	exists, err := file.Exists(accountsFilePath, file.Regular)

	if err != nil {
		log.WithError(err).Errorf("Could not check if file exists: %s", accountsFilePath)
		return
	}

	if !exists {
		log.Warnf("Starting without accounts located in wallet at %s", accountsFilePath)
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.WithError(err).Error("Could not initialize file watcher")
		return
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			log.WithError(err).Error("Could not close file watcher")
		}
	}()
	watchDir := filepath.Clean(filepath.Dir(accountsFilePath))
	accountsFilePath = filepath.Clean(accountsFilePath)
	if err := watcher.Add(watchDir); err != nil {
		log.WithError(err).Errorf("Could not add directory %s to file watcher", watchDir)
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	fileChangesChan := make(chan any, 1)

	// We debounce events sent over the file changes channel by an interval
	// to ensure we are not overwhelmed by a ton of events fired over the channel in
	// a short span of time.
	go async.Debounce(ctx, debounceFileChangesInterval, fileChangesChan, func(any) {
		km.reloadAccountsFromKeystoreFile(accountsFilePath)
	})
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			eventPath := filepath.Clean(event.Name)
			if eventPath == watchDir && (event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)) {
				log.Errorf("Accounts directory was removed: %s", watchDir)
				return
			}
			if eventPath != accountsFilePath {
				continue
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) && !event.Has(fsnotify.Remove) {
				continue
			}
			select {
			case fileChangesChan <- struct{}{}:
			default:
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.WithError(err).Errorf("Could not watch for file changes for: %s", accountsFilePath)
		case <-ctx.Done():
			return
		}
	}
}

func (km *Keymanager) reloadAccountsFromKeystoreFile(accountsFilePath string) {
	if km.wallet == nil {
		log.Error("Could not reload accounts because wallet was undefined")
		return
	}
	fileBytes, err := os.ReadFile(filepath.Clean(accountsFilePath))
	if err != nil {
		log.WithError(err).Errorf("Could not read file at path: %s", accountsFilePath)
		return
	}
	if fileBytes == nil {
		log.WithError(err).Errorf("Loaded in an empty file: %s", accountsFilePath)
		return
	}
	accountsKeystore := &AccountsKeystoreRepresentation{}
	if err := json.Unmarshal(fileBytes, accountsKeystore); err != nil {
		log.WithError(
			err,
		).Errorf("Could not read valid, EIP-2335 keystore json file at path: %s", accountsFilePath)
		return
	}
	if err := km.reloadAccountsFromKeystore(accountsKeystore); err != nil {
		log.WithError(
			err,
		).Error("Could not replace the accounts store from keystore file")
	}
}

// Replaces the accounts store struct in the local keymanager with
// the contents of a keystore file by decrypting it with the accounts password.
func (km *Keymanager) reloadAccountsFromKeystore(keystore *AccountsKeystoreRepresentation) error {
	decryptor := keystorev4.New()
	encodedAccounts, err := decryptor.Decrypt(keystore.Crypto, km.wallet.Password())
	if err != nil {
		return errors.Wrap(err, "could not decrypt keystore file")
	}
	newAccountsStore := &accountStore{}
	if err := json.Unmarshal(encodedAccounts, newAccountsStore); err != nil {
		return err
	}
	if len(newAccountsStore.PublicKeys) != len(newAccountsStore.PrivateKeys) {
		return errors.New("number of public and private keys in keystore do not match")
	}

	pubKeys := make([][fieldparams.BLSPubkeyLength]byte, len(newAccountsStore.PublicKeys))
	for i := 0; i < len(newAccountsStore.PrivateKeys); i++ {
		privKey, err := bls.SecretKeyFromBytes(newAccountsStore.PrivateKeys[i])
		if err != nil {
			return errors.Wrap(err, "could not initialize private key")
		}
		pubKeyBytes := privKey.PublicKey().Marshal()
		pubKeys[i] = bytesutil.ToBytes48(pubKeyBytes)
	}
	km.accountsStore = newAccountsStore
	if err := km.initializeKeysCachesFromKeystore(); err != nil {
		return err
	}
	log.Info(keymanager.KeysReloaded)
	km.accountsChangedFeed.Send(pubKeys)
	return nil
}
