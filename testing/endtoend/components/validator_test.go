package components

import (
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/wallet"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/local"
)

func TestCreateValidatorWallet(t *testing.T) {
	local.ResetCaches()
	defer local.ResetCaches()
	privKey, err := bls.RandKey()
	require.NoError(t, err)
	privKeys := []bls.SecretKey{privKey}
	pubKeys := []bls.PublicKey{privKey.PublicKey()}

	walletDir := t.TempDir()

	// Remove old encrypted wallet files if they exist, to ensure a clean state.
	_, err = createValidatorWallet(t.Context(), walletDir, privKeys, pubKeys)
	require.NoError(t, err)

	passwordFile, err := createValidatorWallet(t.Context(), walletDir, privKeys, pubKeys)
	require.NoError(t, err)
	password, err := os.ReadFile(passwordFile)
	require.NoError(t, err)
	require.Equal(t, 64, len(password))

	w, err := wallet.OpenWallet(t.Context(), &wallet.Config{WalletDir: walletDir, WalletPassword: string(password)})
	require.NoError(t, err)
	km, err := local.NewKeymanager(t.Context(), &local.SetupConfig{Wallet: w})
	require.NoError(t, err)
	keys, err := km.FetchValidatingPublicKeys(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	require.DeepEqual(t, pubKeys[0].Marshal(), keys[0][:])
}
