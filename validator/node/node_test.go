package node

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/prysmaticlabs/prysm/v5/cmd/validator/flags"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/accounts"
	"github.com/prysmaticlabs/prysm/v5/validator/accounts/wallet"
	"github.com/prysmaticlabs/prysm/v5/validator/db/kv"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager"
	remoteweb3signer "github.com/prysmaticlabs/prysm/v5/validator/keymanager/remote-web3signer"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"
)

// Test that the sharding node can build with default flag values.
func TestNode_Builds(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String("datadir", t.TempDir()+"/datadir", "the node data directory")
	dir := t.TempDir() + "/walletpath"
	passwordDir := t.TempDir() + "/password"
	require.NoError(t, os.MkdirAll(passwordDir, os.ModePerm))
	passwordFile := filepath.Join(passwordDir, "password.txt")
	walletPassword := "$$Passw0rdz2$$"
	require.NoError(t, os.WriteFile(
		passwordFile,
		[]byte(walletPassword),
		os.ModePerm,
	))
	set.String("wallet-dir", dir, "path to wallet")
	set.String("wallet-password-file", passwordFile, "path to wallet password")
	set.String("keymanager-kind", "imported", "keymanager kind")
	set.String("verbosity", "debug", "log verbosity")
	require.NoError(t, set.Set(flags.WalletPasswordFileFlag.Name, passwordFile))
	ctx := cli.NewContext(&app, set, nil)
	opts := []accounts.Option{
		accounts.WithWalletDir(dir),
		accounts.WithKeymanagerType(keymanager.Local),
		accounts.WithWalletPassword(walletPassword),
		accounts.WithSkipMnemonicConfirm(true),
	}
	acc, err := accounts.NewCLIManager(opts...)
	require.NoError(t, err)
	_, err = acc.WalletCreate(ctx.Context)
	require.NoError(t, err)

	valClient, err := NewValidatorClient(ctx)
	require.NoError(t, err, "Failed to create ValidatorClient")
	err = valClient.db.Close()
	require.NoError(t, err)
}

func TestGetLegacyDatabaseLocation(t *testing.T) {
	dataDir := t.TempDir()
	dataFile := path.Join(dataDir, "dataFile")
	nonExistingDataFile := path.Join(dataDir, "nonExistingDataFile")
	_, err := os.Create(dataFile)
	require.NoError(t, err, "Failed to create data file")

	walletDir := t.TempDir()
	derivedDir := path.Join(walletDir, "derived")
	err = file.MkdirAll(derivedDir)
	require.NoError(t, err, "Failed to create derived dir")

	derivedDbFile := path.Join(derivedDir, kv.ProtectionDbFileName)
	_, err = os.Create(derivedDbFile)
	require.NoError(t, err, "Failed to create derived db file")

	dbFile := path.Join(walletDir, kv.ProtectionDbFileName)
	_, err = os.Create(dbFile)
	require.NoError(t, err, "Failed to create db file")

	nonExistingWalletDir := t.TempDir()

	testCases := []struct {
		name                      string
		isInteropNumValidatorsSet bool
		isWeb3SignerURLFlagSet    bool
		dataDir                   string
		dataFile                  string
		walletDir                 string
		validatorClient           *ValidatorClient
		wallet                    *wallet.Wallet
		expectedDataDir           string
		expectedDataFile          string
	}{
		{
			name:                      "interop num validators set",
			isInteropNumValidatorsSet: true,
			dataDir:                   dataDir,
			dataFile:                  dataFile,
			expectedDataDir:           dataDir,
			expectedDataFile:          dataFile,
		},
		{
			name:             "dataDir differs from default",
			dataDir:          dataDir,
			dataFile:         dataFile,
			expectedDataDir:  dataDir,
			expectedDataFile: dataFile,
		},
		{
			name:             "dataFile exists",
			dataDir:          cmd.DefaultDataDir(),
			dataFile:         dataFile,
			expectedDataDir:  cmd.DefaultDataDir(),
			expectedDataFile: dataFile,
		},
		{
			name:             "wallet is nil",
			dataDir:          cmd.DefaultDataDir(),
			dataFile:         nonExistingDataFile,
			expectedDataDir:  cmd.DefaultDataDir(),
			expectedDataFile: nonExistingDataFile,
		},
		{
			name:     "web3signer url is not set and legacy data file does not exist",
			dataDir:  cmd.DefaultDataDir(),
			dataFile: nonExistingDataFile,
			wallet: wallet.New(&wallet.Config{
				WalletDir:      nonExistingWalletDir,
				KeymanagerKind: keymanager.Derived,
			}),
			expectedDataDir:  cmd.DefaultDataDir(),
			expectedDataFile: nonExistingDataFile,
		},
		{
			name:     "web3signer url is not set and legacy data file does exist",
			dataDir:  cmd.DefaultDataDir(),
			dataFile: nonExistingDataFile,
			wallet: wallet.New(&wallet.Config{
				WalletDir:      walletDir,
				KeymanagerKind: keymanager.Derived,
			}),
			expectedDataDir:  path.Join(walletDir, "derived"),
			expectedDataFile: path.Join(walletDir, "derived", kv.ProtectionDbFileName),
		},
		{
			name:                   "web3signer url is set and legacy data file does not exist",
			isWeb3SignerURLFlagSet: true,
			dataDir:                cmd.DefaultDataDir(),
			dataFile:               nonExistingDataFile,
			walletDir:              nonExistingWalletDir,
			wallet: wallet.New(&wallet.Config{
				WalletDir:      walletDir,
				KeymanagerKind: keymanager.Derived,
			}),
			expectedDataDir:  cmd.DefaultDataDir(),
			expectedDataFile: nonExistingDataFile,
		},
		{
			name:                   "web3signer url is set and legacy data file does exist",
			isWeb3SignerURLFlagSet: true,
			dataDir:                cmd.DefaultDataDir(),
			dataFile:               nonExistingDataFile,
			walletDir:              walletDir,
			wallet: wallet.New(&wallet.Config{
				WalletDir:      walletDir,
				KeymanagerKind: keymanager.Derived,
			}),
			expectedDataDir:  walletDir,
			expectedDataFile: path.Join(walletDir, kv.ProtectionDbFileName),
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			validatorClient := &ValidatorClient{wallet: tt.wallet}
			actualDataDir, actualDataFile, err := validatorClient.getLegacyDatabaseLocation(
				tt.isInteropNumValidatorsSet,
				tt.isWeb3SignerURLFlagSet,
				tt.dataDir,
				tt.dataFile,
				tt.walletDir,
			)

			require.NoError(t, err, "Failed to get legacy database location")

			assert.Equal(t, tt.expectedDataDir, actualDataDir, "data dir should be equal")
			assert.Equal(t, tt.expectedDataFile, actualDataFile, "data file should be equal")
		})

	}

}

// TestClearDB tests clearing the database
func TestClearDB(t *testing.T) {
	for _, isMinimalDatabase := range []bool{false, true} {
		t.Run(fmt.Sprintf("isMinimalDatabase=%v", isMinimalDatabase), func(t *testing.T) {
			hook := logtest.NewGlobal()
			tmp := filepath.Join(t.TempDir(), "datadirtest")
			require.NoError(t, clearDB(context.Background(), tmp, true, isMinimalDatabase))
			require.LogsContain(t, hook, "Removing database")
		})
	}
}

// TestWeb3SignerConfig tests the web3 signer config returns the correct values.
func TestWeb3SignerConfig(t *testing.T) {
	type args struct {
		baseURL          string
		publicKeysOrURLs []string
		persistentFile   string
	}
	tests := []struct {
		name       string
		args       *args
		want       *remoteweb3signer.SetupConfig
		wantErrMsg string
	}{
		{
			name: "happy path with public keys",
			args: &args{
				baseURL: "http://localhost:8545",
				publicKeysOrURLs: []string{"0xa99a76ed7796f7be22d5b7e85deeb7c5677e88e511e0b337618f8c4eb61349b4bf2d153f649f7b53359fe8b94a38e44c," +
					"0xb89bebc699769726a318c8e9971bd3171297c61aea4a6578a7a4f94b547dcba5bac16a89108b6b6a1fe3695d1a874a0b"},
			},
			want: &remoteweb3signer.SetupConfig{
				BaseEndpoint:          "http://localhost:8545",
				GenesisValidatorsRoot: nil,
				PublicKeysURL:         "",
				ProvidedPublicKeys: []string{
					"0xa99a76ed7796f7be22d5b7e85deeb7c5677e88e511e0b337618f8c4eb61349b4bf2d153f649f7b53359fe8b94a38e44c",
					"0xb89bebc699769726a318c8e9971bd3171297c61aea4a6578a7a4f94b547dcba5bac16a89108b6b6a1fe3695d1a874a0b",
				},
			},
		},
		{
			name: "happy path with external url",
			args: &args{
				baseURL:          "http://localhost:8545",
				publicKeysOrURLs: []string{"http://localhost:8545/api/v1/eth2/publicKeys"},
			},
			want: &remoteweb3signer.SetupConfig{
				BaseEndpoint:          "http://localhost:8545",
				GenesisValidatorsRoot: nil,
				PublicKeysURL:         "http://localhost:8545/api/v1/eth2/publicKeys",
				ProvidedPublicKeys:    nil,
			},
		},
		{
			name: "Bad base URL",
			args: &args{
				baseURL: "0xa99a76ed7796f7be22d5b7e85deeb7c5677e88,",
				publicKeysOrURLs: []string{"0xa99a76ed7796f7be22d5b7e85deeb7c5677e88e511e0b337618f8c4eb61349b4bf2d153f649f7b53359fe8b94a38e44c," +
					"0xb89bebc699769726a318c8e9971bd3171297c61aea4a6578a7a4f94b547dcba5bac16a89108b6b6a1fe3695d1a874a0b"},
			},
			want:       nil,
			wantErrMsg: "web3signer url 0xa99a76ed7796f7be22d5b7e85deeb7c5677e88, is invalid: parse \"0xa99a76ed7796f7be22d5b7e85deeb7c5677e88,\": invalid URI for request",
		},
		{
			name: "Base URL missing scheme or host",
			args: &args{
				baseURL:          "localhost:8545",
				publicKeysOrURLs: []string{"localhost"},
			},
			want:       nil,
			wantErrMsg: "web3signer url must be in the format of http(s)://host:port url used: localhost:8545",
		},
		{
			name: "happy path with persistentFile",
			args: &args{
				baseURL:        "http://localhost:8545",
				persistentFile: "/remote/key/file.txt",
			},
			want: &remoteweb3signer.SetupConfig{
				BaseEndpoint: "http://localhost:8545",
				KeyFilePath:  "/remote/key/file.txt",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := cli.App{}
			set := flag.NewFlagSet(tt.name, 0)
			set.String("validators-external-signer-url", tt.args.baseURL, "baseUrl")
			set.String(flags.Web3SignerKeyFileFlag.Name, "", "")
			c := &cli.StringSliceFlag{
				Name: "validators-external-signer-public-keys",
			}
			err := c.Apply(set)
			require.NoError(t, err)
			require.NoError(t, set.Set(flags.Web3SignerURLFlag.Name, tt.args.baseURL))
			for _, key := range tt.args.publicKeysOrURLs {
				require.NoError(t, set.Set(flags.Web3SignerPublicValidatorKeysFlag.Name, key))
			}
			if tt.args.persistentFile != "" {
				require.NoError(t, set.Set(flags.Web3SignerKeyFileFlag.Name, tt.args.persistentFile))
			}
			cliCtx := cli.NewContext(&app, set, nil)
			got, err := Web3SignerConfig(cliCtx)
			if tt.wantErrMsg != "" {
				require.ErrorContains(t, tt.wantErrMsg, err)
				return
			}
			require.DeepEqual(t, tt.want, got)
		})
	}
}
