package accounts

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/prysmaticlabs/prysm/v5/build/bazel"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	validatormock "github.com/prysmaticlabs/prysm/v5/testing/validator-mock"
	"github.com/prysmaticlabs/prysm/v5/validator/accounts"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestExitAccountsCli_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockValidatorClient := validatormock.NewMockValidatorClient(ctrl)
	mockNodeClient := validatormock.NewMockNodeClient(ctrl)

	mockValidatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), gomock.Any()).
		Return(&ethpb.ValidatorIndexResponse{Index: 1}, nil)

	// Any time in the past will suffice
	genesisTime := &timestamppb.Timestamp{
		Seconds: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
	}

	mockNodeClient.EXPECT().
		Genesis(gomock.Any(), gomock.Any()).
		Return(&ethpb.Genesis{GenesisTime: genesisTime}, nil)

	mockValidatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	mockValidatorClient.EXPECT().
		ProposeExit(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.SignedVoluntaryExit{})).
		Return(&ethpb.ProposeExitResponse{}, nil)

	walletDir, _, passwordFilePath := setupWalletAndPasswordsDir(t)
	// Write a directory where we will import keys from.
	keysDir := filepath.Join(t.TempDir(), "keysDir")
	require.NoError(t, os.MkdirAll(keysDir, os.ModePerm))

	// Create keystore file in the keys directory we can then import from in our wallet.
	keystore, _ := createKeystore(t, keysDir)
	time.Sleep(time.Second)

	// We initialize a wallet with a local keymanager.
	cliCtx := setupWalletCtx(t, &testWalletConfig{
		// Wallet configuration flags.
		walletDir:           walletDir,
		keymanagerKind:      keymanager.Local,
		walletPasswordFile:  passwordFilePath,
		accountPasswordFile: passwordFilePath,
		// Flag required for ImportAccounts to work.
		keysDir: keysDir,
		// Flag required for ExitAccounts to work.
		voluntaryExitPublicKeys: keystore.Pubkey,
	})
	opts := []accounts.Option{
		accounts.WithWalletDir(walletDir),
		accounts.WithKeymanagerType(keymanager.Local),
		accounts.WithWalletPassword(password),
	}
	acc, err := accounts.NewCLIManager(opts...)
	require.NoError(t, err)
	_, err = acc.WalletCreate(cliCtx.Context)
	require.NoError(t, err)
	require.NoError(t, accountsImport(cliCtx))

	_, km, err := walletWithKeymanager(cliCtx)
	require.NoError(t, err)
	require.NotNil(t, km)

	validatingPublicKeys, err := km.FetchValidatingPublicKeys(cliCtx.Context)
	require.NoError(t, err)
	require.NotNil(t, validatingPublicKeys)

	// Prepare user input for final confirmation step
	var stdin bytes.Buffer
	stdin.Write([]byte("Y"))
	rawPubKeys, formattedPubKeys, err := accounts.FilterExitAccountsFromUserInput(
		cliCtx, &stdin, validatingPublicKeys, false,
	)
	require.NoError(t, err)
	require.NotNil(t, rawPubKeys)
	require.NotNil(t, formattedPubKeys)

	cfg := accounts.PerformExitCfg{
		ValidatorClient:  mockValidatorClient,
		NodeClient:       mockNodeClient,
		Keymanager:       km,
		RawPubKeys:       rawPubKeys,
		FormattedPubKeys: formattedPubKeys,
	}
	rawExitedKeys, formattedExitedKeys, err := accounts.PerformVoluntaryExit(cliCtx.Context, cfg)
	require.NoError(t, err)
	require.Equal(t, 1, len(rawExitedKeys))
	assert.DeepEqual(t, rawPubKeys[0], rawExitedKeys[0])
	require.Equal(t, 1, len(formattedExitedKeys))
	assert.Equal(t, "0x"+keystore.Pubkey[:12], formattedExitedKeys[0])
}

func TestExitAccountsCli_OK_AllPublicKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockValidatorClient := validatormock.NewMockValidatorClient(ctrl)
	mockNodeClient := validatormock.NewMockNodeClient(ctrl)

	mockValidatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), gomock.Any()).
		Return(&ethpb.ValidatorIndexResponse{Index: 0}, nil)

	mockValidatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), gomock.Any()).
		Return(&ethpb.ValidatorIndexResponse{Index: 1}, nil)

	// Any time in the past will suffice
	genesisTime := &timestamppb.Timestamp{
		Seconds: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
	}

	mockNodeClient.EXPECT().
		Genesis(gomock.Any(), gomock.Any()).
		Return(&ethpb.Genesis{GenesisTime: genesisTime}, nil)

	mockValidatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Times(2).
		Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	mockValidatorClient.EXPECT().
		ProposeExit(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.SignedVoluntaryExit{})).
		Times(2).
		Return(&ethpb.ProposeExitResponse{}, nil)

	walletDir, _, passwordFilePath := setupWalletAndPasswordsDir(t)
	// Write a directory where we will import keys from.
	keysDir := filepath.Join(t.TempDir(), "keysDir")
	require.NoError(t, os.MkdirAll(keysDir, os.ModePerm))

	// Create keystore file in the keys directory we can then import from in our wallet.
	keystore1, _ := createKeystore(t, keysDir)
	time.Sleep(time.Second)
	keystore2, _ := createKeystore(t, keysDir)
	time.Sleep(time.Second)

	// We initialize a wallet with a local keymanager.
	cliCtx := setupWalletCtx(t, &testWalletConfig{
		// Wallet configuration flags.
		walletDir:           walletDir,
		keymanagerKind:      keymanager.Local,
		walletPasswordFile:  passwordFilePath,
		accountPasswordFile: passwordFilePath,
		// Flag required for ImportAccounts to work.
		keysDir: keysDir,
		// Exit all public keys.
		exitAll: true,
	})
	opts := []accounts.Option{
		accounts.WithWalletDir(walletDir),
		accounts.WithKeymanagerType(keymanager.Local),
		accounts.WithWalletPassword(password),
	}
	acc, err := accounts.NewCLIManager(opts...)
	require.NoError(t, err)
	_, err = acc.WalletCreate(cliCtx.Context)
	require.NoError(t, err)
	require.NoError(t, accountsImport(cliCtx))

	_, km, err := walletWithKeymanager(cliCtx)
	require.NoError(t, err)
	require.NotNil(t, km)

	validatingPublicKeys, err := km.FetchValidatingPublicKeys(cliCtx.Context)
	require.NoError(t, err)
	require.NotNil(t, validatingPublicKeys)

	// Prepare user input for final confirmation step
	var stdin bytes.Buffer
	stdin.Write([]byte("Y"))
	rawPubKeys, formattedPubKeys, err := accounts.FilterExitAccountsFromUserInput(
		cliCtx, &stdin, validatingPublicKeys, false,
	)
	require.NoError(t, err)
	require.NotNil(t, rawPubKeys)
	require.NotNil(t, formattedPubKeys)

	cfg := accounts.PerformExitCfg{
		ValidatorClient:  mockValidatorClient,
		NodeClient:       mockNodeClient,
		Keymanager:       km,
		RawPubKeys:       rawPubKeys,
		FormattedPubKeys: formattedPubKeys,
	}
	rawExitedKeys, formattedExitedKeys, err := accounts.PerformVoluntaryExit(cliCtx.Context, cfg)
	require.NoError(t, err)
	require.Equal(t, 2, len(rawExitedKeys))
	assert.DeepEqual(t, rawPubKeys, rawExitedKeys)
	require.Equal(t, 2, len(formattedExitedKeys))
	wantedFormatted := []string{
		"0x" + keystore1.Pubkey[:12],
		"0x" + keystore2.Pubkey[:12],
	}
	sort.Strings(wantedFormatted)
	sort.Strings(formattedExitedKeys)
	require.DeepEqual(t, wantedFormatted, formattedExitedKeys)
}

func TestExitAccountsCli_OK_ForceExit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockValidatorClient := validatormock.NewMockValidatorClient(ctrl)
	mockNodeClient := validatormock.NewMockNodeClient(ctrl)

	mockValidatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), gomock.Any()).
		Return(&ethpb.ValidatorIndexResponse{Index: 1}, nil)

	// Any time in the past will suffice
	genesisTime := &timestamppb.Timestamp{
		Seconds: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
	}

	mockNodeClient.EXPECT().
		Genesis(gomock.Any(), gomock.Any()).
		Return(&ethpb.Genesis{GenesisTime: genesisTime}, nil)

	mockValidatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	mockValidatorClient.EXPECT().
		ProposeExit(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.SignedVoluntaryExit{})).
		Return(&ethpb.ProposeExitResponse{}, nil)

	walletDir, _, passwordFilePath := setupWalletAndPasswordsDir(t)
	// Write a directory where we will import keys from.
	keysDir := filepath.Join(t.TempDir(), "keysDir")
	require.NoError(t, os.MkdirAll(keysDir, os.ModePerm))

	// Create keystore file in the keys directory we can then import from in our wallet.
	keystore, _ := createKeystore(t, keysDir)
	time.Sleep(time.Second)

	// We initialize a wallet with a local keymanager.
	cliCtx := setupWalletCtx(t, &testWalletConfig{
		// Wallet configuration flags.
		walletDir:           walletDir,
		keymanagerKind:      keymanager.Local,
		walletPasswordFile:  passwordFilePath,
		accountPasswordFile: passwordFilePath,
		// Flag required for ImportAccounts to work.
		keysDir: keysDir,
		// Flag required for ExitAccounts to work.
		voluntaryExitPublicKeys: keystore.Pubkey,
	})
	opts := []accounts.Option{
		accounts.WithWalletDir(walletDir),
		accounts.WithKeymanagerType(keymanager.Local),
		accounts.WithWalletPassword(password),
	}
	acc, err := accounts.NewCLIManager(opts...)
	require.NoError(t, err)
	_, err = acc.WalletCreate(cliCtx.Context)
	require.NoError(t, err)
	require.NoError(t, accountsImport(cliCtx))

	_, km, err := walletWithKeymanager(cliCtx)
	require.NoError(t, err)
	require.NotNil(t, km)

	validatingPublicKeys, err := km.FetchValidatingPublicKeys(cliCtx.Context)
	require.NoError(t, err)
	require.NotNil(t, validatingPublicKeys)

	rawPubKeys, formattedPubKeys, err := accounts.FilterExitAccountsFromUserInput(
		cliCtx, &bytes.Buffer{}, validatingPublicKeys, true,
	)
	require.NoError(t, err)
	require.NotNil(t, rawPubKeys)
	require.NotNil(t, formattedPubKeys)

	cfg := accounts.PerformExitCfg{
		ValidatorClient:  mockValidatorClient,
		NodeClient:       mockNodeClient,
		Keymanager:       km,
		RawPubKeys:       rawPubKeys,
		FormattedPubKeys: formattedPubKeys,
	}
	rawExitedKeys, formattedExitedKeys, err := accounts.PerformVoluntaryExit(cliCtx.Context, cfg)
	require.NoError(t, err)
	require.Equal(t, 1, len(rawExitedKeys))
	assert.DeepEqual(t, rawPubKeys[0], rawExitedKeys[0])
	require.Equal(t, 1, len(formattedExitedKeys))
	assert.Equal(t, "0x"+keystore.Pubkey[:12], formattedExitedKeys[0])
}

func TestExitAccountsCli_WriteJSON_NoBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockValidatorClient := validatormock.NewMockValidatorClient(ctrl)
	mockNodeClient := validatormock.NewMockNodeClient(ctrl)

	mockValidatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), gomock.Any()).
		Return(&ethpb.ValidatorIndexResponse{Index: 1}, nil)

	// Any time in the past will suffice
	genesisTime := &timestamppb.Timestamp{
		Seconds: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
	}

	mockNodeClient.EXPECT().
		Genesis(gomock.Any(), gomock.Any()).
		Return(&ethpb.Genesis{GenesisTime: genesisTime}, nil)

	mockValidatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	walletDir, _, passwordFilePath := setupWalletAndPasswordsDir(t)
	// Write a directory where we will import keys from.
	keysDir := filepath.Join(t.TempDir(), "keysDir")
	require.NoError(t, os.MkdirAll(keysDir, os.ModePerm))

	// Create keystore file in the keys directory we can then import from in our wallet.
	keystore, _ := createKeystore(t, keysDir)
	time.Sleep(time.Second)

	// We initialize a wallet with a local keymanager.
	cliCtx := setupWalletCtx(t, &testWalletConfig{
		// Wallet configuration flags.
		walletDir:           walletDir,
		keymanagerKind:      keymanager.Local,
		walletPasswordFile:  passwordFilePath,
		accountPasswordFile: passwordFilePath,
		// Flag required for ImportAccounts to work.
		keysDir: keysDir,
		// Flag required for ExitAccounts to work.
		voluntaryExitPublicKeys: keystore.Pubkey,
	})
	opts := []accounts.Option{
		accounts.WithWalletDir(walletDir),
		accounts.WithKeymanagerType(keymanager.Local),
		accounts.WithWalletPassword(password),
	}
	acc, err := accounts.NewCLIManager(opts...)
	require.NoError(t, err)
	_, err = acc.WalletCreate(cliCtx.Context)
	require.NoError(t, err)
	require.NoError(t, accountsImport(cliCtx))

	_, km, err := walletWithKeymanager(cliCtx)
	require.NoError(t, err)
	require.NotNil(t, km)

	validatingPublicKeys, err := km.FetchValidatingPublicKeys(cliCtx.Context)
	require.NoError(t, err)
	require.NotNil(t, validatingPublicKeys)

	rawPubKeys, formattedPubKeys, err := accounts.FilterExitAccountsFromUserInput(
		cliCtx, &bytes.Buffer{}, validatingPublicKeys, true,
	)
	require.NoError(t, err)
	require.NotNil(t, rawPubKeys)
	require.NotNil(t, formattedPubKeys)

	out := path.Join(bazel.TestTmpDir(), "exits")

	cfg := accounts.PerformExitCfg{
		ValidatorClient:  mockValidatorClient,
		NodeClient:       mockNodeClient,
		Keymanager:       km,
		RawPubKeys:       rawPubKeys,
		FormattedPubKeys: formattedPubKeys,
		OutputDirectory:  out,
	}
	rawExitedKeys, formattedExitedKeys, err := accounts.PerformVoluntaryExit(cliCtx.Context, cfg)
	require.NoError(t, err)
	require.Equal(t, 1, len(rawExitedKeys))
	assert.DeepEqual(t, rawPubKeys[0], rawExitedKeys[0])
	require.Equal(t, 1, len(formattedExitedKeys))
	assert.Equal(t, "0x"+keystore.Pubkey[:12], formattedExitedKeys[0])

	exists, err := file.Exists(path.Join(out, "validator-exit-1.json"), file.Regular)
	require.NoError(t, err, "could not check if exit file exists")
	require.Equal(t, true, exists, "Expected file to exist")
}
