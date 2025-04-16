// package mock
//
// lint:nopanic -- Test / mock code, allowed to panic.
package mock

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/OffchainLabs/prysm/v6/validator/accounts/iface"
	"github.com/OffchainLabs/prysm/v6/validator/keymanager"
)

// Wallet contains an in-memory, simulated wallet implementation.
type Wallet struct {
	InnerAccountsDir  string
	Directories       []string
	Files             map[string]map[string][]byte
	EncryptedSeedFile []byte
	AccountPasswords  map[string]string
	WalletPassword    string
	UnlockAccounts    bool
	lock              sync.RWMutex
	HasWriteFileError bool
	WalletDir         string
	Kind              keymanager.Kind
}

// AccountNames --
func (w *Wallet) AccountNames() ([]string, error) {
	w.lock.RLock()
	defer w.lock.RUnlock()
	names := make([]string, 0)
	for name := range w.AccountPasswords {
		names = append(names, name)
	}
	return names, nil
}

// AccountsDir --
func (w *Wallet) AccountsDir() string {
	return w.InnerAccountsDir
}

// Dir for the wallet.
func (w *Wallet) Dir() string {
	return w.WalletDir
}

// KeymanagerKind --
func (w *Wallet) KeymanagerKind() keymanager.Kind {
	return w.Kind
}

// Exists --
func (w *Wallet) Exists() (bool, error) {
	return len(w.Directories) > 0, nil
}

// Password --
func (w *Wallet) Password() string {
	return w.WalletPassword
}

// WriteFileAtPath --
func (w *Wallet) WriteFileAtPath(_ context.Context, pathName, fileName string, data []byte) (bool, error) {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.HasWriteFileError {
		// reset the flag to not contaminate other tests
		w.HasWriteFileError = false
		return false, errors.New("could not write keystore file for accounts")
	}
	if w.Files[pathName] == nil {
		w.Files[pathName] = make(map[string][]byte)
	}
	w.Files[pathName][fileName] = data
	return true, nil
}

// ReadFileAtPath --
func (w *Wallet) ReadFileAtPath(_ context.Context, pathName, fileName string) ([]byte, error) {
	w.lock.RLock()
	defer w.lock.RUnlock()
	for f, v := range w.Files[pathName] {
		if strings.Contains(fileName, f) {
			return v, nil
		}
	}
	return nil, errors.New("no files found")
}

// InitializeKeymanager --
func (_ *Wallet) InitializeKeymanager(_ context.Context, _ iface.InitKeymanagerConfig) (keymanager.IKeymanager, error) {
	return nil, nil
}
