package deposit

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/pkg/errors"
)

// parsed ABI cached at package level to avoid reparsing on every log unpack
var depositABI abi.ABI

func init() {
	var err error
	depositABI, err = abi.JSON(strings.NewReader(DepositContractABI))
	if err != nil {
		// DepositContractABI is a constant generated from the contract; failure here is unrecoverable
		panic(err)
	}
}

// UnpackDepositLogData unpacks the data from a deposit log using the ABI decoder.
func UnpackDepositLogData(data []byte) (pubkey, withdrawalCredentials, amount, signature, index []byte, err error) {
	evt := new(DepositContractDepositEvent)
	if err := depositABI.UnpackIntoInterface(evt, "DepositEvent", data); err != nil {
		return nil, nil, nil, nil, nil, errors.Wrap(err, "unable to unpack logs")
	}
	return evt.Pubkey, evt.WithdrawalCredentials, evt.Amount, evt.Signature, evt.Index, nil
}
