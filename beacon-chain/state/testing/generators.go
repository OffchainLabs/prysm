package testing

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

// GeneratePendingDeposit is used for testing and producing a signed pending deposit
func GeneratePendingDeposit(t *testing.T, key common.SecretKey, amount uint64, withdrawalCredentials [32]byte, slot primitives.Slot) *ethpb.PendingDeposit {
	dm := &ethpb.DepositMessage{
		PublicKey:             key.PublicKey().Marshal(),
		WithdrawalCredentials: withdrawalCredentials[:],
		Amount:                amount,
	}
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	sr, err := signing.ComputeSigningRoot(dm, domain)
	require.NoError(t, err)
	sig := key.Sign(sr[:])
	depositData := &ethpb.Deposit_Data{
		PublicKey:             bytesutil.SafeCopyBytes(dm.PublicKey),
		WithdrawalCredentials: bytesutil.SafeCopyBytes(dm.WithdrawalCredentials),
		Amount:                dm.Amount,
		Signature:             sig.Marshal(),
	}
	valid, err := blocks.IsValidDepositSignature(depositData)
	require.NoError(t, err)
	require.Equal(t, true, valid)
	return &ethpb.PendingDeposit{
		PublicKey:             bytesutil.SafeCopyBytes(dm.PublicKey),
		WithdrawalCredentials: bytesutil.SafeCopyBytes(dm.WithdrawalCredentials),
		Amount:                dm.Amount,
		Signature:             sig.Marshal(),
		Slot:                  slot,
	}
}
