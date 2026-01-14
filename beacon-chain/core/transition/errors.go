package transition

import "errors"

var (
	ErrAttestationsSignatureInvalid          = errors.New("attestations signature invalid")
	ErrRandaoSignatureInvalid                = errors.New("randao signature invalid")
	ErrBLSToExecutionChangesSignatureInvalid = errors.New("BLS to execution changes signature invalid")
	ErrProcessWithdrawalsFailed              = errors.New("process withdrawals failed")
	ErrProcessRandaoFailed                   = errors.New("process randao failed")
	ErrProcessEth1DataFailed                 = errors.New("process eth1 data failed")
	ErrProcessProposerSlashingsFailed        = errors.New("process proposer slashings failed")
	ErrProcessAttesterSlashingsFailed        = errors.New("process attester slashings failed")
	ErrProcessAttestationsFailed             = errors.New("process attestations failed")
	ErrProcessDepositsFailed                 = errors.New("process deposits failed")
	ErrProcessVoluntaryExitsFailed           = errors.New("process voluntary exits failed")
	ErrProcessBLSChangesFailed               = errors.New("process BLS to execution changes failed")
	ErrProcessSyncAggregateFailed            = errors.New("process sync aggregate failed")
)
