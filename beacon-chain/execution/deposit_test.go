package execution

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestDepositContractAddress_EmptyAddress(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.DepositContractAddress = ""
	params.OverrideBeaconConfig(config)

	_, err := DepositContractAddress()
	assert.ErrorContains(t, "valid deposit contract is required", err)
}

func TestDepositContractAddress_NotHexAddress(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.DepositContractAddress = "abc?!"
	params.OverrideBeaconConfig(config)

	_, err := DepositContractAddress()
	assert.ErrorContains(t, "invalid deposit contract address given", err)
}

func TestDepositContractAddress_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	addr, err := DepositContractAddress()
	require.NoError(t, err)
	assert.Equal(t, params.BeaconConfig().DepositContractAddress, addr)
}
