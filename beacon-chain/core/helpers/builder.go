package helpers

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
)

// IsBuilderWithdrawalCredential returns true if the withdrawal credentials indicate a builder.
func IsBuilderWithdrawalCredential(withdrawalCredentials []byte) bool {
	return len(withdrawalCredentials) == fieldparams.RootLength &&
		withdrawalCredentials[0] == params.BeaconConfig().BuilderWithdrawalPrefixByte
}
