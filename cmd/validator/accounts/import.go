package accounts

import (
	"strings"

	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v6/validator/accounts"
	"github.com/OffchainLabs/prysm/v6/validator/accounts/iface"
	"github.com/OffchainLabs/prysm/v6/validator/accounts/userprompt"
	"github.com/OffchainLabs/prysm/v6/validator/accounts/wallet"
	"github.com/OffchainLabs/prysm/v6/validator/client"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

func accountsImport(c *cli.Context) error {
	w, err := walletImport(c)
	if err != nil {
		return errors.Wrap(err, "could not initialize wallet")
	}
	km, err := w.InitializeKeymanager(c.Context, iface.InitKeymanagerConfig{ListenForChanges: false})
	if err != nil {
		return err
	}

	dialOpts := client.ConstructDialOptions(
		c.Int(cmd.GrpcMaxCallRecvMsgSizeFlag.Name),
		c.String(flags.CertFlag.Name),
		c.Uint(flags.GRPCRetriesFlag.Name),
		c.Duration(flags.GRPCRetryDelayFlag.Name),
	)
	grpcHeaders := strings.Split(c.String(flags.GRPCHeadersFlag.Name), ",")

	opts := []accounts.Option{
		accounts.WithWallet(w),
		accounts.WithKeymanager(km),
		accounts.WithGRPCDialOpts(dialOpts),
		accounts.WithBeaconRPCProvider(c.String(flags.BeaconRPCProviderFlag.Name)),
		accounts.WithBeaconRESTApiProvider(c.String(flags.BeaconRESTApiProviderFlag.Name)),
		accounts.WithGRPCHeaders(grpcHeaders),
	}

	opts = append(opts, accounts.WithImportPrivateKeys(c.IsSet(flags.ImportPrivateKeyFileFlag.Name)))
	opts = append(opts, accounts.WithPrivateKeyFile(c.String(flags.ImportPrivateKeyFileFlag.Name)))
	opts = append(opts, accounts.WithReadPasswordFile(c.IsSet(flags.AccountPasswordFileFlag.Name)))
	opts = append(opts, accounts.WithPasswordFilePath(c.String(flags.AccountPasswordFileFlag.Name)))

	keysDir, err := userprompt.InputDirectory(c, userprompt.ImportKeysDirPromptText, flags.KeysDirFlag)
	if err != nil {
		return errors.Wrap(err, "could not parse keys directory")
	}
	opts = append(opts, accounts.WithKeysDir(keysDir))

	acc, err := accounts.NewCLIManager(opts...)
	if err != nil {
		return err
	}
	return acc.Import(c.Context)
}

func walletImport(c *cli.Context) (*wallet.Wallet, error) {
	return wallet.OpenWalletOrElseCli(c, wallet.OpenOrCreateNewWallet)
}
