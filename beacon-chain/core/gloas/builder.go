package gloas

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
)

// IsBuilderWithdrawalCredential returns true when the builder withdrawal prefix is set.
// Spec v1.6.1 (pseudocode):
// def is_builder_withdrawal_credential(withdrawal_credentials: Bytes32) -> bool:
//
//	return withdrawal_credentials[:1] == BUILDER_WITHDRAWAL_PREFIX
func IsBuilderWithdrawalCredential(withdrawalCredentials []byte) bool {
	if len(withdrawalCredentials) == 0 {
		return false
	}
	return withdrawalCredentials[0] == params.BeaconConfig().BuilderWithdrawalPrefixByte
}
