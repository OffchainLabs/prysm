package eth

import "github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"

// Copy --
func (pd *PendingDeposit) Copy() *PendingDeposit {
	if pd == nil {
		return nil
	}
	return &PendingDeposit{
		PublicKey:             bytesutil.SafeCopyBytes(pd.PublicKey),
		WithdrawalCredentials: bytesutil.SafeCopyBytes(pd.WithdrawalCredentials),
		Amount:                pd.Amount,
		Signature:             bytesutil.SafeCopyBytes(pd.Signature),
		Slot:                  pd.Slot,
	}
}

// Copy --
func (pw *PendingPartialWithdrawal) Copy() *PendingPartialWithdrawal {
	if pw == nil {
		return nil
	}
	return &PendingPartialWithdrawal{
		Index:             pw.Index,
		Amount:            pw.Amount,
		WithdrawableEpoch: pw.WithdrawableEpoch,
	}
}

// Copy --
func (pc *PendingConsolidation) Copy() *PendingConsolidation {
	if pc == nil {
		return nil
	}
	return &PendingConsolidation{
		SourceIndex: pc.SourceIndex,
		TargetIndex: pc.TargetIndex,
	}
}
