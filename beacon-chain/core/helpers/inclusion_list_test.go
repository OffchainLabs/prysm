package helpers_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

func TestValidateNilInclusionList(t *testing.T) {
	tests := []struct {
		name        string
		il          *eth.InclusionList
		errContains string
	}{
		{
			name:        "nil inclusion list",
			il:          nil,
			errContains: "nil inclusion list",
		},
		{
			name:        "nil committee root",
			il:          &eth.InclusionList{},
			errContains: "nil inclusion list committee root",
		},
		{
			name: "valid",
			il:   &eth.InclusionList{InclusionListCommitteeRoot: make([]byte, 32)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := helpers.ValidateNilInclusionList(tt.il)
			if tt.errContains == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, tt.errContains, err)
		})
	}
}

func TestValidateNilSignedInclusionList(t *testing.T) {
	tests := []struct {
		name        string
		il          *eth.SignedInclusionList
		errContains string
	}{
		{
			name:        "nil signed inclusion list",
			il:          nil,
			errContains: "nil inclusion list",
		},
		{
			name:        "nil signature",
			il:          &eth.SignedInclusionList{Message: &eth.InclusionList{InclusionListCommitteeRoot: make([]byte, 32)}},
			errContains: "nil signature",
		},
		{
			name:        "nil message",
			il:          &eth.SignedInclusionList{Signature: make([]byte, 96)},
			errContains: "nil inclusion list",
		},
		{
			name:        "nil committee root",
			il:          &eth.SignedInclusionList{Signature: make([]byte, 96), Message: &eth.InclusionList{}},
			errContains: "nil inclusion list committee root",
		},
		{
			name: "valid",
			il: &eth.SignedInclusionList{
				Signature: make([]byte, 96),
				Message:   &eth.InclusionList{InclusionListCommitteeRoot: make([]byte, 32)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := helpers.ValidateNilSignedInclusionList(tt.il)
			if tt.errContains == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, tt.errContains, err)
		})
	}
}

func TestGetInclusionListCommittee_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.Eip7805ForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	st, _ := util.DeterministicGenesisStateElectra(t, 256)

	committee, err := helpers.GetInclusionListCommittee(t.Context(), st, 0)
	require.NoError(t, err)
	require.Equal(t, int(params.BeaconConfig().InclusionListCommitteeSize), len(committee))

	numValidators := st.NumValidators()
	for _, idx := range committee {
		require.Equal(t, true, uint64(idx) < uint64(numValidators))
	}

	committee2, err := helpers.GetInclusionListCommittee(t.Context(), st, 0)
	require.NoError(t, err)
	require.DeepEqual(t, committee, committee2)
}

func TestGetInclusionListCommittee_BeforeFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.Eip7805ForkEpoch = 5
	params.OverrideBeaconConfig(cfg)

	st, _ := util.DeterministicGenesisStateElectra(t, 256)

	_, err := helpers.GetInclusionListCommittee(t.Context(), st, 0)
	require.ErrorContains(t, "incorrect state version", err)
}

func TestValidateInclusionListSignature(t *testing.T) {
	st, privKeys := util.DeterministicGenesisStateElectra(t, 64)

	valIdx := primitives.ValidatorIndex(3)
	msg := &eth.InclusionList{
		Slot:                       0,
		ValidatorIndex:             valIdx,
		InclusionListCommitteeRoot: make([]byte, 32),
		Transactions:               [][]byte{[]byte("tx1")},
	}

	epoch := slots.ToEpoch(st.Slot())
	domain, err := signing.Domain(st.Fork(), epoch, params.BeaconConfig().DomainInclusionListCommittee, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(msg, domain)
	require.NoError(t, err)

	t.Run("valid signature", func(t *testing.T) {
		sig := privKeys[valIdx].Sign(root[:])
		signed := &eth.SignedInclusionList{Message: msg, Signature: sig.Marshal()}
		require.NoError(t, helpers.ValidateInclusionListSignature(t.Context(), st, signed))
	})

	t.Run("wrong signer fails verification", func(t *testing.T) {
		// Signed by a different validator than msg.ValidatorIndex, so verification must fail.
		sig := privKeys[valIdx+1].Sign(root[:])
		signed := &eth.SignedInclusionList{Message: msg, Signature: sig.Marshal()}
		err := helpers.ValidateInclusionListSignature(t.Context(), st, signed)
		require.NotNil(t, err)
	})

	t.Run("nil signed inclusion list", func(t *testing.T) {
		err := helpers.ValidateInclusionListSignature(t.Context(), st, nil)
		require.ErrorContains(t, "nil inclusion list", err)
	})
}
