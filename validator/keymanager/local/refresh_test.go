package local

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	mock "github.com/OffchainLabs/prysm/v7/validator/accounts/testing"
	"github.com/google/uuid"
	keystorev4 "github.com/wealdtech/go-eth2-wallet-encryptor-keystorev4"
)

func TestListenForAccountChanges_AtomicReplacement(t *testing.T) {
	const debounce = 50 * time.Millisecond
	resetFeatures := features.InitWithReset(&features.Flags{KeystoreImportDebounceInterval: debounce})
	defer resetFeatures()
	ResetCaches()
	defer ResetCaches()

	password := "Passw03rdz293**%#2"
	wallet := &mock.Wallet{
		InnerAccountsDir: filepath.Join(t.TempDir(), "wallet"),
		WalletPassword:   password,
	}
	km := &Keymanager{
		wallet:              wallet,
		accountsStore:       &accountStore{},
		accountsChangedFeed: new(event.Feed),
	}
	encodeStore := func(key bls.SecretKey) ([]byte, [fieldparams.BLSPubkeyLength]byte) {
		pubkey := key.PublicKey().Marshal()
		store := &accountStore{
			PrivateKeys: [][]byte{key.Marshal()},
			PublicKeys:  [][]byte{pubkey},
		}
		representation, err := CreateAccountsKeystoreRepresentation(t.Context(), store, password)
		require.NoError(t, err)
		encoded, err := json.Marshal(representation)
		require.NoError(t, err)
		return encoded, bytesutil.ToBytes48(pubkey)
	}
	keyA, err := bls.RandKey()
	require.NoError(t, err)
	keyB, err := bls.RandKey()
	require.NoError(t, err)
	encodedA, pubkeyA := encodeStore(keyA)
	encodedB, pubkeyB := encodeStore(keyB)
	accountsDir := filepath.Join(wallet.AccountsDir(), AccountsPath)
	require.NoError(t, file.MkdirAll(accountsDir))
	accountsFilePath := filepath.Join(accountsDir, AccountsKeystoreFileName)
	require.NoError(t, file.WriteFile(accountsFilePath, encodedA))
	km.reloadAccountsFromKeystoreFile(accountsFilePath)

	changes := make(chan [][fieldparams.BLSPubkeyLength]byte, 4)
	sub := km.SubscribeAccountChanges(changes)
	defer sub.Unsubscribe()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go km.listenForAccountChanges(ctx)
	time.Sleep(2 * debounce)

	require.NoError(t, file.WriteFileAtomically(accountsFilePath, encodedB))
	requireLocalAccountChange(t, changes, pubkeyB)
	require.NoError(t, file.WriteFileAtomically(accountsFilePath, encodedA))
	requireLocalAccountChange(t, changes, pubkeyA)
}

func requireLocalAccountChange(t *testing.T, changes <-chan [][fieldparams.BLSPubkeyLength]byte, want [fieldparams.BLSPubkeyLength]byte) {
	t.Helper()
	select {
	case keys := <-changes:
		require.Equal(t, 1, len(keys))
		require.Equal(t, want, keys[0])
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for local account change")
	}
}

func TestLocalKeymanager_reloadAccountsFromKeystore_MismatchedNumKeys(t *testing.T) {
	password := "Passw03rdz293**%#2"
	wallet := &mock.Wallet{
		Files:            make(map[string]map[string][]byte),
		AccountPasswords: make(map[string]string),
		WalletPassword:   password,
	}
	dr := &Keymanager{
		wallet: wallet,
	}
	accountsStore := &accountStore{
		PrivateKeys: [][]byte{[]byte("hello")},
		PublicKeys:  [][]byte{[]byte("hi"), []byte("world")},
	}
	encodedStore, err := json.MarshalIndent(accountsStore, "", "\t")
	require.NoError(t, err)
	encryptor := keystorev4.New()
	cryptoFields, err := encryptor.Encrypt(encodedStore, dr.wallet.Password())
	require.NoError(t, err)
	id, err := uuid.NewRandom()
	require.NoError(t, err)
	keystore := &AccountsKeystoreRepresentation{
		Crypto:  cryptoFields,
		ID:      id.String(),
		Version: encryptor.Version(),
		Name:    encryptor.Name(),
	}
	err = dr.reloadAccountsFromKeystore(keystore)
	assert.ErrorContains(t, "do not match", err)
}

func TestLocalKeymanager_reloadAccountsFromKeystore(t *testing.T) {
	password := "Passw03rdz293**%#2"
	wallet := &mock.Wallet{
		Files:            make(map[string]map[string][]byte),
		AccountPasswords: make(map[string]string),
		WalletPassword:   password,
	}
	dr := &Keymanager{
		wallet:              wallet,
		accountsChangedFeed: new(event.Feed),
	}

	numAccounts := 20
	privKeys := make([][]byte, numAccounts)
	pubKeys := make([][]byte, numAccounts)
	for i := range numAccounts {
		privKey, err := bls.RandKey()
		require.NoError(t, err)
		privKeys[i] = privKey.Marshal()
		pubKeys[i] = privKey.PublicKey().Marshal()
	}

	accountsStore, err := dr.CreateAccountsKeystore(t.Context(), privKeys, pubKeys)
	require.NoError(t, err)
	require.NoError(t, dr.reloadAccountsFromKeystore(accountsStore))

	// Check that the public keys were added to the public keys cache.
	for i, keyBytes := range pubKeys {
		require.Equal(t, bytesutil.ToBytes48(keyBytes), orderedPublicKeys[i])
	}

	// Check that the secret keys were added to the secret keys cache.
	lock.RLock()
	defer lock.RUnlock()
	for i, keyBytes := range privKeys {
		privKey, ok := secretKeysCache[bytesutil.ToBytes48(pubKeys[i])]
		require.Equal(t, true, ok)
		require.Equal(t, bytesutil.ToBytes48(keyBytes), bytesutil.ToBytes48(privKey.Marshal()))
	}

	// Check the key was added to the global accounts store.
	require.Equal(t, numAccounts, len(dr.accountsStore.PublicKeys))
	require.Equal(t, numAccounts, len(dr.accountsStore.PrivateKeys))
	assert.DeepEqual(t, dr.accountsStore.PublicKeys[0], pubKeys[0])
}
