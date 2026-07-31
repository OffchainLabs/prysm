package accounts

import (
	"strings"

	"github.com/OffchainLabs/prysm/v7/validator/accounts"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/iface"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/wallet"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

func walletWithKeymanager(c *cli.Context) (*wallet.Wallet, keymanager.IKeymanager, error) {
	w, err := wallet.OpenWalletOrElseCli(c, func(cliCtx *cli.Context) (*wallet.Wallet, error) {
		return nil, wallet.ErrNoWalletFound
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not open wallet")
	}
	km, err := w.InitializeKeymanager(c.Context, iface.InitKeymanagerConfig{ListenForChanges: false})
	if err != nil && strings.Contains(err.Error(), keymanager.IncorrectPasswordErrMsg) {
		return nil, nil, errors.New("wrong wallet password entered")
	}
	if err != nil {
		return nil, nil, errors.Wrap(err, accounts.ErrCouldNotInitializeKeymanager)
	}
	return w, km, nil
}
