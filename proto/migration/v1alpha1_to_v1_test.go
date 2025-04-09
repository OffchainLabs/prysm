package migration

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpbv1 "github.com/prysmaticlabs/prysm/v5/proto/eth/v1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func Test_V1ValidatorToV1Alpha1(t *testing.T) {
	v1Validator := &ethpbv1.Validator{
		Pubkey:                     []byte("pubkey"),
		WithdrawalCredentials:      []byte("withdraw"),
		EffectiveBalance:           99,
		Slashed:                    true,
		ActivationEligibilityEpoch: 1,
		ActivationEpoch:            11,
		ExitEpoch:                  111,
		WithdrawableEpoch:          1111,
	}

	v1Alpha1Validator := V1ValidatorToV1Alpha1(v1Validator)
	require.NotNil(t, v1Alpha1Validator)
	assert.DeepEqual(t, []byte("pubkey"), v1Alpha1Validator.PublicKey)
	assert.DeepEqual(t, []byte("withdraw"), v1Alpha1Validator.WithdrawalCredentials)
	assert.Equal(t, uint64(99), v1Alpha1Validator.EffectiveBalance)
	assert.Equal(t, true, v1Alpha1Validator.Slashed)
	assert.Equal(t, primitives.Epoch(1), v1Alpha1Validator.ActivationEligibilityEpoch)
	assert.Equal(t, primitives.Epoch(11), v1Alpha1Validator.ActivationEpoch)
	assert.Equal(t, primitives.Epoch(111), v1Alpha1Validator.ExitEpoch)
	assert.Equal(t, primitives.Epoch(1111), v1Alpha1Validator.WithdrawableEpoch)
}
