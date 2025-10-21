package helpers

import (
	"bytes"

	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

func ForksEqual(s, t *ethpb.Fork) bool {
	if s == nil && t == nil {
		return true
	}
	if s == nil || t == nil {
		return false
	}
	if s.Epoch != t.Epoch {
		return false
	}
	if !bytes.Equal(s.PreviousVersion, t.PreviousVersion) {
		return false
	}
	return bytes.Equal(s.CurrentVersion, t.CurrentVersion)
}

func BlockHeadersEqual(s, t *ethpb.BeaconBlockHeader) bool {
	if s == nil && t == nil {
		return true
	}
	if s == nil || t == nil {
		return false
	}
	if s.Slot != t.Slot {
		return false
	}
	if s.ProposerIndex != t.ProposerIndex {
		return false
	}
	if !bytes.Equal(s.ParentRoot, t.ParentRoot) {
		return false
	}
	if !bytes.Equal(s.StateRoot, t.StateRoot) {
		return false
	}
	return bytes.Equal(s.BodyRoot, t.BodyRoot)
}

func Eth1DataEqual(s, t *ethpb.Eth1Data) bool {
	if s == nil && t == nil {
		return true
	}
	if s == nil || t == nil {
		return false
	}
	if !bytes.Equal(s.DepositRoot, t.DepositRoot) {
		return false
	}
	if s.DepositCount != t.DepositCount {
		return false
	}
	return bytes.Equal(s.BlockHash, t.BlockHash)
}

func PendingDepositsEqual(s, t *ethpb.PendingDeposit) bool {
	if s == nil && t == nil {
		return true
	}
	if s == nil || t == nil {
		return false
	}
	if !bytes.Equal(s.PublicKey, t.PublicKey) {
		return false
	}
	if !bytes.Equal(s.WithdrawalCredentials, t.WithdrawalCredentials) {
		return false
	}
	if s.Amount != t.Amount {
		return false
	}
	if !bytes.Equal(s.Signature, t.Signature) {
		return false
	}
	return s.Slot == t.Slot
}

func PendingPartialWithdrawalsEqual(s, t *ethpb.PendingPartialWithdrawal) bool {
	if s == nil && t == nil {
		return true
	}
	if s == nil || t == nil {
		return false
	}
	if s.Index != t.Index {
		return false
	}
	if s.Amount != t.Amount {
		return false
	}
	return s.WithdrawableEpoch == t.WithdrawableEpoch
}

func PendingConsolidationsEqual(s, t *ethpb.PendingConsolidation) bool {
	if s == nil && t == nil {
		return true
	}
	if s == nil || t == nil {
		return false
	}
	return s.SourceIndex == t.SourceIndex && s.TargetIndex == t.TargetIndex
}
